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
	"os"
	"strconv"
	"strings"

	"go-hep.org/x/hep/hplot"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

func main() {
	http.HandleFunc("/", rootHandle)
	http.HandleFunc("/img-confirmed", imgHandle("cases", 100))
	http.HandleFunc("/img-deaths", imgHandle("deaths", 10))
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

		f, err := os.Create("covid-" + strings.ToLower(title) + ".png")
		if err != nil {
			log.Printf("error: %+v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		defer f.Close()
		err = png.Encode(f, img)
		if err != nil {
			log.Printf("error: %+v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		"United States",
		"United Kingdom",
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
		line.Width = 2
		p.Add(line)
		p.Legend.Add(fmt.Sprintf("%s %8d", name, int(ys[len(ys)-1])), line)
	}
	fct := hplot.NewFunction(func(x float64) float64 {
		return cutoff * math.Pow(1.33, x)
	})
	fct.LineStyle.Color = color.Gray16{}
	fct.LineStyle.Width = 2
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
	url := fmt.Sprintf("https://covid.ourworldindata.org/data/total_%s.csv", title)
	//url := fmt.Sprintf("https://raw.githubusercontent.com/CSSEGISandData/COVID-19/master/csse_covid_19_data/csse_covid_19_time_series/time_series_19-covid-%s.csv", title)

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
	n2i := make(map[string]int, len(countries))
	for i, name := range hdr {
		n2i[name] = i
	}
	delete(n2i, "date")
	for _, name := range countries {
		if _, ok := n2i[name]; !ok {
			return "", nil, fmt.Errorf("could not find country %q in dataset", name)
		}
	}

	dataset := make(map[string][]float64, len(countries))
	date := ""

loop:
	for {
		rec, err := raw.Read()
		if err != nil {
			if err == io.EOF {
				break loop
			}
			return "", nil, fmt.Errorf("could not read CSV data: %w", err)
		}
		date = rec[0]
		for name, i := range n2i {
			str := rec[i]
			if str == "" {
				dataset[name] = append(dataset[name], 0)
				continue
			}
			v, err := strconv.ParseFloat(str, 64)
			if err != nil {
				return "", nil, fmt.Errorf("could not parse %q: %w", str, err)
			}
			dataset[name] = append(dataset[name], v)
		}
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

	return date, dataset, nil
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
