// Copyright 2020 The covid19 Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/csv"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"go-hep.org/x/hep/hplot"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

func main() {
	http.HandleFunc("/", rootHandle)
	http.HandleFunc("/img-confirmed", imgHandle("Confirmed", 100))
	http.HandleFunc("/img-deaths", imgHandle("Deaths", 10))
	http.ListenAndServe(":8080", nil)
}

func rootHandle(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, page)
}

func imgHandle(title string, cutoff float64) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		img, err := genImage(title, cutoff)
		if err != nil {
			log.Printf("error: %+v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = png.Encode(w, img)
		if err != nil {
			log.Printf("error: %+v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func genImage(title string, cutoff float64) (image.Image, error) {
	countries := []string{
		"France",
		"Italy",
		"Spain",
		//	"Korea, South",
		//	"China",
		"Germany",
	}
	date, dataset, err := fetchData(title, cutoff, countries)
	if err != nil {
		return nil, fmt.Errorf("could not fetch data: %w", err)
	}
	log.Printf("%s: data for %q", title, date)

	p := hplot.New()
	p.Title.Text = "CoVid-19 - " + title + " - " + date
	p.X.Label.Text = fmt.Sprintf("Days from first %d confirmed cases", int(cutoff))
	p.Y.Scale = plot.LogScale{}
	p.Y.Tick.Marker = plot.LogTicks{}

	for i, name := range countries {
		ys := dataset[name]
		xs := make([]float64, len(ys))
		for i := range xs {
			xs[i] = float64(i)
		}
		xys := hplot.ZipXY(xs, ys)
		line, err := hplot.NewLine(xys)
		if err != nil {
			return nil, fmt.Errorf("could not create line plot for %q: %w", name, err)
		}
		line.Color = plotutil.SoftColors[i]
		p.Add(line)
		p.Legend.Add(fmt.Sprintf("%s %8d", name, int(ys[len(ys)-1])), line)
	}
	fct := hplot.NewFunction(func(x float64) float64 {
		return cutoff * math.Pow(1.33, x)
	})
	fct.LineStyle.Color = color.Gray16{}
	fct.LineStyle.Dashes = plotutil.Dashes(1)
	p.Add(fct)
	p.Legend.Add("33% daily growth", fct)
	p.Add(hplot.NewGrid())

	const sz = 20 * vg.Centimeter
	cnv := vgimg.PngCanvas{vgimg.New(sz*math.Phi, sz)}

	c := draw.New(cnv)
	p.Draw(c)
	return cnv.Image(), nil
}

func fetchData(title string, cutoff float64, countries []string) (string, map[string][]float64, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/CSSEGISandData/COVID-19/master/csse_covid_19_data/csse_covid_19_time_series/time_series_19-covid-%s.csv", title)

	resp, err := http.Get(url)
	if err != nil {
		return "", nil, fmt.Errorf("could not retrieve data file: %w", err)
	}
	defer resp.Body.Close()

	raw := csv.NewReader(resp.Body)
	raw.Comma = ','

	hdr, err := raw.Read()
	if err != nil {
		return "", nil, fmt.Errorf("could not read CSV header: %w", err)
	}

	sz := len(hdr) - 4
	dataset := make(map[string][]float64, len(countries))
	for _, name := range countries {
		dataset[name] = make([]float64, sz)
	}

loop:
	for {
		rec, err := raw.Read()
		if err != nil {
			if err == io.EOF {
				break loop
			}
			return "", nil, fmt.Errorf("could not read CSV data: %w", err)
		}

		if _, ok := dataset[rec[1]]; !ok {
			continue
		}

		name := rec[1]
		rec = rec[4:]
		data := make([]float64, len(rec))
		for i, str := range rec {
			v, err := strconv.ParseFloat(str, 64)
			if err != nil {
				return "", nil, fmt.Errorf("could not parse %q: %w", str, err)
			}
			data[i] = v
		}
		floats.Add(dataset[name], data)
	}

	for _, name := range countries {
		data := dataset[name]
		idx := 0
	cleanup:
		for i, v := range data {
			if v >= cutoff {
				idx = i
				break cleanup
			}
		}
		dataset[name] = data[idx:]
	}

	const layout = "1/2/06"
	date, err := time.Parse(layout, hdr[len(hdr)-1])
	if err != nil {
		return "", nil, fmt.Errorf("could not parse date: %w", err)
	}

	return date.Format("2006-01-02"), dataset, nil
}

const page = `<!DOCTYPE html>
<html>
	<head>
		<title>COVID-19</title>
	</head>
	<body>
		<div id="content">
			<img id="plot" src="/img-confirmed"/>
			<img id="plot" src="/img-deaths"/>
		</div>
	</body>
</html>
`
