// Copyright ©2016 Jonathan J Lawlor. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// benchplot interactively fits and displays a least squares fit on groups of parameterized benchmarks.
//
// Usage:
//
//	benchplot [options] bench1.txt [bench2.txt ...]
//
// The input bench.txt file(s) should contain the output of a number of runs of
// ``go test -bench.'' Benchmarks that match the regexp in the ``vars'' flag
// will be collected into a sample for fitting a least squares regression.
//
// Example
//
// Suppose we collect benchmark results from running ``go test -bench=Sort''
// on this package.
//
// The file bench.txt contains:
//
//   PASS
//   BenchmarkSort10-4            	 1000000	      1008 ns/op
//   BenchmarkSort100-4           	  200000	      8224 ns/op
//   BenchmarkSort1000-4          	   10000	    152945 ns/op
//   BenchmarkSort10000-4         	    1000	   1950999 ns/op
//   BenchmarkSort100000-4        	      50	  25081946 ns/op
//   BenchmarkSort1000000-4       	       5	 302228845 ns/op
//   BenchmarkSort10000000-4      	       1	3631295293 ns/op
//   BenchmarkStableSort10-4      	 1000000	      1260 ns/op
//   BenchmarkStableSort100-4     	  100000	     16730 ns/op
//   BenchmarkStableSort1000-4    	    5000	    362024 ns/op
//   BenchmarkStableSort10000-4   	     300	   5731738 ns/op
//   BenchmarkStableSort100000-4  	      20	  88171712 ns/op
//   BenchmarkStableSort1000000-4 	       1	1205361782 ns/op
//   BenchmarkStableSort10000000-4	       1	14349613704 ns/op
//   ok  	github.com/jonlawlor/benchplot	138.860s
//
// In these benchmarks, the suffix 10 .. 10000000 indicates how many items are
// sorted in the benchmark.  benchplot can estimate and interactively visualize
// the relationship between the number of elements to sort and how long it
// takes to perform the sort.
//
// Options are:
//    -http=addr
//       HTTP service address (e.g., '127.0.0.1:6060' or just ':6060')
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/gonum/matrix/mat64"
	"github.com/jonlawlor/parsefloat"
	"golang.org/x/tools/benchmark/parse"
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: benchplot [options] bench1.txt [bench2.txt ...]\n")
	fmt.Fprintf(os.Stderr, "interactively fits and displays a least squares fit on parameterized benchmarks\n")
	fmt.Fprintf(os.Stderr, "example:\n")
	fmt.Fprintf(os.Stderr, "   benchplot -http=:8080 bench.txt")
	fmt.Fprintf(os.Stderr, "options:\n")
	flag.PrintDefaults()
	os.Exit(2)
}

const (
	defaultAddr = ":6060" // default webserver address
)

var (
	httpAddr = flag.String("http", defaultAddr, "HTTP service address (e.g., '"+defaultAddr+"')")
	verbose  = flag.Bool("v", false, "verbose mode")
)

// validYs has the Y name as keys and a human readable name as the value.
var validYs = map[string]string{
	"NsPerOp":           "ns/op",
	"AllocedBytesPerOp": "B/op",
	"AllocsPerOp":       "allocs/op",
	"MBPerS":            "MB/s"}

func main() {
	log.SetPrefix("benchplot: ")
	log.SetFlags(0)
	flag.Usage = usage
	flag.Parse()

	// Evaluate the glob args to see if any of them are malformed.  We don't read
	// any of the files at this time.  This is the only error that Glob can return,
	// so this allows benchplot to fail fast.
	for _, arg := range flag.Args() {
		if _, err := filepath.Glob(arg); err != nil {
			log.Fatalf("invalid benchmark filename: %s", arg)
		}
	}

	dataHandleFunc := serveBenchmarksAsJSON(flag.Args())

	var handler http.Handler = http.DefaultServeMux
	if *verbose {
		log.Printf("version = %s", runtime.Version())
		log.Printf("address = %s", *httpAddr)
		handler = loggingHandler(handler)
	}

	// Add the benchmark data handler.   It serves up the benchmark data in json
	// form at /data
	http.Handle("/data", dataHandleFunc)

	// Add the plotter.  It fetches data from /data, filters it, sends it to
	// /fit, and displays the results.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.CopyBuffer(w, strings.NewReader(plotHTML), nil)
	})

	// Fit takes requests with a querystring describing the function to fit,
	// and a set of data within a put, along with desired bounds for the estimation.
	// It returns a set of points and the 95% confidence interval in JSON.
	http.HandleFunc("/fit", fitHandleFunc)

	if err := http.ListenAndServe(*httpAddr, handler); err != nil {
		log.Fatalf("ListenAndServe %s: %v", *httpAddr, err)
	}

}

func loggingHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		log.Printf("%s\t%s", req.RemoteAddr, req.URL)
		h.ServeHTTP(w, req)
	})
}

func serveBenchmarksAsJSON(patterns []string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		benchSets := make(map[string][]*parse.Benchmark)
		for _, pat := range patterns {
			// we've already checked for validity, so err will be nil
			fns, _ := filepath.Glob(pat)
			for _, fn := range fns {
				// This can only error if the path is invalid but glob should only return
				// files that exist.  There's a race condition with the filesystem, but
				// we'll ignore it.
				f, err := os.Open(fn)
				if err != nil {
					continue
				}
				benchSet, err := parse.ParseSet(f)

				if err != nil {
					// TODO(jonlawlor): determine if and when this can occur?
					log.Fatal(err)
				}
				var benchMarks []*parse.Benchmark
				for _, b := range benchSet {
					benchMarks = append(benchMarks, b...)
				}
				benchSets[fn] = benchMarks
			}
		}
		enc := json.NewEncoder(w)
		enc.Encode(benchSets)
	})
}

type benchmarkResponse struct {
	parse.Benchmark
	X float64 // explanatory variable
}

func fitHandleFunc(w http.ResponseWriter, r *http.Request) {
	// TODO(jonlawlor): do something better than fatal logging when there is
	// an invalid input?  Ideally the javascript would never provide invalid data.

	// pull out the fitting parameters from the url querystring
	if err := r.ParseForm(); err != nil {
		log.Fatal(err)
	}

	// lower bound
	xlbValue := r.FormValue("xlb")
	xlb, err := strconv.ParseFloat(xlbValue, 64)
	if err != nil {
		log.Fatal("Invalid x lower bound:", xlbValue)
	}

	// upper bound
	xubValue := r.FormValue("xub")
	xub, err := strconv.ParseFloat(xubValue, 64)
	if err != nil {
		log.Fatal("Invalid x upper bound:", xubValue)
	}

	// x transform
	xTransformValue := r.FormValue("xtransform")

	// create the x expression
	varNames := map[string]struct{}{"N": struct{}{}}
	xTransform, err := parsefloat.NewSlice("float64{"+xTransformValue+"}", varNames)
	if err != nil {
		log.Fatal("invalid xTransform", xTransformValue)
	}

	// response
	yVar := r.FormValue("yvar")

	// number of steps to evaluate
	nLineStepsValue := r.FormValue("nlinesteps")
	nLineSteps, err := strconv.Atoi(nLineStepsValue)
	if err != nil || nLineSteps < 1 {
		log.Fatal("invalid number of line steps:", nLineStepsValue)
	}

	// Unmarshal the data set
	var benchSet []benchmarkResponse
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Fatal("Unable to read request body:", r)
	}
	json.Unmarshal(b, &benchSet)

	// evaluate the regression
	samp := sampleGroup(benchSet, xTransform, yVar)
	regModel := estimate(samp)

	// generate the regression line and the confidence interval
	evalStep := (xub - xlb) / float64(nLineSteps-1)
	evalPoints := make([]float64, nLineSteps)
	point := xlb
	for i := 0; i < nLineSteps; i++ {
		evalPoints[i] = point
		point += evalStep
	}
	regX := evaluate(xTransform, evalPoints)
	betas := mat64.NewDense(len(regModel), 1, regModel)

	var regLine mat64.Dense
	regLine.Mul(regX, betas)

	// generate the regression stats
	r2, mse, bint, iXTX := stats(regModel, samp)

	// evaluate the confidence interval
	confWidth := make([]float64, nLineSteps)
	dof := len(benchSet) - len(xTransform)
	for i := range confWidth {
		xi := regX.RowView(i)
		confWidth[i] = conf95(math.Sqrt(mse*mat64.Inner(xi, iXTX, xi)), dof)
	}

	// pack up the results and respond
	type resultPoint struct {
		X         float64
		Yhat      float64
		ConfWidth float64
	}
	resultLine := make([]resultPoint, nLineSteps)
	for i, x := range evalPoints {
		resultLine[i] = resultPoint{x, regLine.At(i, 0), confWidth[i]}
	}

	type resultModel struct {
		XTrans string
		Beta   float64
		BInt   float64
	}
	resModel := make([]resultModel, len(xTransform))
	for i, x := range xTransform {
		resModel[i] = resultModel{x.String(), betas.At(i, 0), bint[i]}
	}

	w.Header().Set("Content-Type", "application/javascript")
	json.NewEncoder(w).Encode(struct {
		ResultLine  []resultPoint
		ResultModel []resultModel
		R2          float64
		MSE         float64
	}{
		resultLine,
		resModel,
		r2,
		mse,
	})
}
