// Copyright ©2016 Jonathan J Lawlor. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// Static content for the plotter.  There is probably a better way to do this,
// with go.rice, http.ServeFile or go generate.

// javascript is partially based on http://bl.ocks.org/weiglemc/6185069,
// and also "Interactive Data Visualization for the Web" by Scott Murray.
// I had never written any javascript before this project.  I'm sure that fills
// you with confidence!

// TODO(jonlawlor): serve d3.js locally so that benchplot works without an
// internet connection.

const plotHTML = `
<!DOCTYPE html>
<html lang="en">
	<head>
		<meta charset="utf-8">
		<title>go benchplot</title>
    <script src="http://d3js.org/d3.v3.min.js" charset="utf-8"></script>
		<style type="text/css">

			.axis path,
			.axis line {
				fill: none;
				stroke: black;
				shape-rendering: crispEdges;
			}

			.axis text {
				font-family: sans-serif;
				font-size: 11px;
			}

      body {
        font: 11px sans-serif;
      }

      .dot {
        stroke: #000;
      }

      .line {
        fill: none;
        stroke: steelblue;
        stroke-width: 1.5px;
      }
      .boundline {
        fill: none;
        stroke: steelblue;
        stroke-dasharray: 10,10;
        stroke-width: 1.5px;
      }

      .tooltip {
        position: absolute;
        width: 200px;
        height: 28px;
        pointer-events: none;
      }
		</style>
	</head>
	<body>
		<script type="text/javascript">
      var w = 600
      var h = 400
      // TODO(jonlawlor): automatically change the lefthand scale from ns/op
      // to a larger period (like ms/op...) so we don't have so many 0's on the
      // left axis.
      var margin = {top: 20, right: 20, bottom: 30, left: 100},
        width = w - margin.left - margin.right,
        height = h - margin.top - margin.bottom;

      // TODO(jonlawlor): allow user to specify response
      var yVar = 'NsPerOp'

      // regex to match the explanatory variable.
      // TODO(jonlawlor): allow user to specify X variable and grouping regexp
      var nre = /^(.*?)\/?(\d+)-\d+$/

      // TODO(jonlawlor): allow user to specify the explanatory function to fit on.
      var xTransform = "math.Log(N) * N, 1.0"

      // the number of points to evaluate for the regressions
      var nLineSteps = 1000

      // setup x
      var xValue = function(d) { return d.X;}, // data -> value
          xScale = d3.scale.linear().range([0, width]), // value -> display
          xMap = function(d) { return xScale(xValue(d));}, // data -> display
          xAxis = d3.svg.axis().scale(xScale).orient("bottom");

      // setup y
      var yValue = function(d) { return d[yVar];}, // data -> value
          yScale = d3.scale.linear().range([height, 0]), // value -> display
          yMap = function(d) { return yScale(yValue(d));}, // data -> display
          yMap = function(d) { return yScale(yValue(d));}, // data -> display
          yAxis = d3.svg.axis().scale(yScale).orient("left");

      // setup regression line, lower bound, upper bound
      var regLine = d3.svg.line()
          .x(function(d) { return xScale(d.X); })
          .y(function(d) { console.log("regLine"); return yScale(d.Yhat); });

      var regLineLB = d3.svg.line()
          .x(function(d) { return xScale(d.X); })
          .y(function(d) { return yScale(d.Yhat + d.ConfWidth); });

      var regLineUB = d3.svg.line()
          .x(function(d) { return xScale(d.X); })
          .y(function(d) { return yScale(d.Yhat - d.ConfWidth); });

      // setup fill color
      var cValue = function(d) { return d.Group;},
          color = d3.scale.category10();

      // add the graph canvas to the body of the webpage
      var svg = d3.select("body").append("svg")
          .attr("width", width + margin.left + margin.right)
          .attr("height", height + margin.top + margin.bottom)
        .append("g")
          .attr("transform", "translate(" + margin.left + "," + margin.top + ")");

      // add the tooltip area to the webpage
      var tooltip = d3.select("body").append("div")
          .attr("class", "tooltip")
          .style("opacity", 0);

      // Group up the data by the "Group" field so that we can send each
      // group of tests to the fitting service independently.
      // Code is adapted from the stackoverflow question:
      // http://stackoverflow.com/questions/15887900/group-objects-by-property-in-javascript
      function groupBy(orig, groupProp) {
        var newArr = [],
        groups = {},
        newItem, i, j, cur;
        for (i = 0, j = orig.length; i < j; i++) {
          cur = orig[i];
          if (!(cur[groupProp] in groups)) {
            groups[cur[groupProp]] = {benchmarks: []};
            groups[cur[groupProp]][groupProp] = cur[groupProp]
            newArr.push(groups[cur[groupProp]]);
            }
          groups[cur[groupProp]].benchmarks.push(orig[i]);
          }
        return newArr;
      }

      // orderBy returns a function which can be used to order an array of
      // javascript objects.
      function orderBy(orderProp) {
        return function(a, b) {
        if (a[orderProp] < b[orderProp])
          return -1;
        else if (a[orderProp] > b[orderProp])
          return 1;
        else
          return 0;
        }
      }

      // regHandler returns a function which can plot regression lines.  It
      // is necessary because we "forget" what group we are using when we get
      // a response from the call to fit.  There is probably a better way to do
      // this kind of currying in javascript.
      function regHandler(Group) {
          return function(error, data) {
          // TODO(jonlawlor): handle error
          // TODO(jonlawlor): do something with model form and model stats
          var linedataset = []
          for (j in data.ResultLine) {
            data.ResultLine[j].X = Number(data.ResultLine[j].X)
            data.ResultLine[j].Yhat = Number(data.ResultLine[j].Yhat)
            data.ResultLine[j].ConfWidth = Number(data.ResultLine[j].ConfWidth)
            linedataset.push(data.ResultLine[j])
            }

          svg.append("path")
            .datum(linedataset)
            .attr("class", "line")
            .attr("d", regLine)
            .style("stroke", function(d) { return color(Group);});

          svg.append("path")
            .datum(linedataset)
            .attr("class", "boundline")
            .attr("d", regLineUB)
            .style("stroke", function(d) { return color(Group);});

          svg.append("path")
            .datum(linedataset)
            .attr("class", "boundline")
            .attr("d", regLineLB)
            .style("stroke", function(d) { return color(Group);});
          }
        }

			//dataset
      d3.json("/data", function(data) {
        var dataset = []
        // extract the dataset
        for (i in data) {
          for (j in data[i]) {
            var matches = data[i][j].Name.match(nre)
            var n;
            if (matches && matches.length > 1) {
              data[i][j].Group = matches[1]
              data[i][j].X = Number(matches[2])
              dataset.push(data[i][j])
              }
            }
          }
        // don't want dots overlapping axis, so add in buffer to data domain
        xScale.domain([d3.min(dataset, xValue)-1, d3.max(dataset, xValue)+1]);
        yScale.domain([d3.min(dataset, yValue)-1, d3.max(dataset, yValue)+1]);

        // sort the benchmark groups in alphabetical order, so that the same set
        // of benchmarks always results in the same coloring.
        dataset.sort(orderBy("Group"))

        // TODO(jonlawlor): allow log scale
        // x-axis
        svg.append("g")
            .attr("class", "x axis")
            .attr("transform", "translate(0," + height + ")")
            .call(xAxis)
          .append("text")
            .attr("class", "label")
            .attr("x", width)
            .attr("y", -6)
            .style("text-anchor", "end")
            .text("N");

        // TODO(jonlawlor): fit long numbers in better
        // y-axis
        svg.append("g")
            .attr("class", "y axis")
            .call(yAxis)
          .append("text")
            .attr("class", "label")
            .attr("transform", "rotate(-90)")
            .attr("y", 6)
            .attr("dy", ".71em")
            .style("text-anchor", "end")
            .text("ns/op");

        // draw dots
        svg.selectAll(".dot")
            .data(dataset)
          .enter().append("circle")
            .attr("class", "dot")
            .attr("r", 3.5)
            .attr("cx", xMap)
            .attr("cy", yMap)
            .style("fill", function(d) { return color(cValue(d));})
            .on("mouseover", function(d) {
                tooltip.transition()
                     .duration(200)
                     .style("opacity", .9);
                tooltip.html(d.Group + "<br/> (" + xValue(d)
      	        + ", " + yValue(d) + ")")
                     .style("left", (d3.event.pageX + 5) + "px")
                     .style("top", (d3.event.pageY - 28) + "px");
            })
            .on("mouseout", function(d) {
                tooltip.transition()
                     .duration(500)
                     .style("opacity", 0);
            });

        var benchGroups = groupBy(dataset, "Group")

        for (i in benchGroups) {
          d3.json("/fit?" +
                  "response=" + encodeURIComponent(yVar) +
                  "&xlb=" + encodeURIComponent(d3.min(dataset, xValue)) +
                  "&xub=" + encodeURIComponent(d3.max(dataset, xValue)) +
                  "&xtransform=" + encodeURIComponent(xTransform) +
                  "&yvar=" + encodeURIComponent(yVar) +
                  "&nlinesteps=" + encodeURIComponent(nLineSteps))
            .header("Content-Type", "application/json")
            .post(JSON.stringify(benchGroups[i].benchmarks), regHandler(benchGroups[i].Group))
          }

        // draw legend
        var legend = svg.selectAll(".legend")
            .data(color.domain())
          .enter().append("g")
            .attr("class", "legend")
            .attr("transform", function(d, i) { return "translate(0," + i * 20 + ")"; });

        // draw legend colored rectangles
        legend.append("rect")
            .attr("x", 30)
            .attr("width", 18)
            .attr("height", 18)
            .style("fill", color);

        // draw legend text
        legend.append("text")
            .attr("x", 52)
            .attr("y", 9)
            .attr("dy", ".35em")
            .text(function(d) { return d;})
        })
		</script>
	</body>
</html>
`
