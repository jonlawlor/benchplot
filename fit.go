// Copyright ©2016 Jonathan J Lawlor. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"log"
	"math"

	"github.com/gonum/blas"
	"github.com/gonum/blas/blas64"
	"github.com/gonum/lapack/lapack64"
	"github.com/gonum/matrix/mat64"
	"github.com/jonlawlor/parsefloat"
)

type samp struct {
	x []float64 // explanatory
	y []float64 // response
}

// sampleGroup finds the samples in the benchmark.  The resulting samp x and y will
// not be in a stable order.
func sampleGroup(benchSet []benchmarkResponse, xExprs []parsefloat.Expression, yVar string) samp {

	// pull out the response
	var y []float64
	switch yVar {
	case "NsPerOp":
		for _, b := range benchSet {
			y = append(y, b.NsPerOp)
		}
	case "AllocedBytesPerOp":
		for _, b := range benchSet {
			y = append(y, float64(b.AllocedBytesPerOp))
		}
	case "AllocsPerOp":
		for _, b := range benchSet {
			y = append(y, float64(b.AllocsPerOp))
		}
	case "MBPerS":
		for _, b := range benchSet {
			y = append(y, b.MBPerS)
		}
	default:
		log.Fatal("unknown YVar:", yVar)
	}

	// construct the explanatory variable
	var x []float64
	for _, bs := range benchSet {
		// convert input string matches into a variable map
		vars := map[string]float64{"N": bs.X}

		// eval x
		for _, xExpr := range xExprs {
			x = append(x, xExpr.Eval(vars))
		}
	}
	return samp{x, y}
}

// model contains the model parameters
type model []float64

// estimate parameters via least squares.  Returns nil if it could not converge.
func estimate(s samp) model {
	y := blas64.General{
		Rows:   len(s.y),
		Cols:   1,
		Stride: 1,
		Data:   make([]float64, len(s.y)),
	}
	copy(y.Data, s.y)

	x := blas64.General{
		Rows:   len(s.y),
		Cols:   len(s.x) / len(s.y),
		Stride: len(s.x) / len(s.y),
		Data:   make([]float64, len(s.x)),
	}
	copy(x.Data, s.x)

	// find optimal work size
	work := make([]float64, 1)
	lapack64.Gels(blas.NoTrans, x, y, work, -1)

	work = make([]float64, int(work[0]))
	ok := lapack64.Gels(blas.NoTrans, x, y, work, len(work))

	if !ok {
		return nil
	}
	return y.Data[:x.Cols]
}

// calculate R squared
func stats(m model, s samp) (r2, mse float64, cint []float64, iXTX *mat64.Dense) {
	RSS := 0.0
	YSS := 0.0

	// also consumed degrees of freedom
	stride := len(s.x) / len(s.y)
	for i, y := range s.y {
		YSS += y * y
		yHat := 0.0
		for j, x := range s.x[i*stride : (i+1)*stride] {
			yHat += m[j] * x
		}
		RSS += (yHat - y) * (yHat - y)
	}
	r2 = 1.0 - RSS/YSS

	mse = RSS / float64(len(s.y)-stride)
	X := mat64.NewDense(len(s.y), stride, s.x)
	iXTX = mat64.NewDense(stride, stride, make([]float64, stride*stride))
	iXTX.Mul(X.T(), X)
	iXTX.Inverse(iXTX)
	cint = make([]float64, stride)
	for i := 0; i < stride; i++ {
		cint[i] = conf95(math.Sqrt(iXTX.At(i, i)*mse), len(s.y)-stride)
	}

	return
}

// evaluate the given expression at the given points, returning values in a
// matrix.
func evaluate(xExprs []parsefloat.Expression, points []float64) *mat64.Dense {
	vars := map[string]float64{"N": 0.0}
	var data []float64
	for _, n := range points {
		vars["N"] = n
		for _, xExpr := range xExprs {
			data = append(data, xExpr.Eval(vars))
		}
	}
	return mat64.NewDense(len(points), len(xExprs), data)
}
