// Copyright 2016-2020 Markus Lindenberg, Stig Bakken
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"strconv"
	"strings"
	"text/scanner"

	"github.com/prometheus/common/log"
)

type metric struct {
	Name  string
	Value float64
}

type labelset struct {
	Names  []string
	Values []string
}

func (l *labelset) Equals(labels []string) bool {
	if len(l.Names) != len(labels) {
		return false
	}
	for i := range l.Names {
		if l.Names[i] != labels[i] {
			return false
		}
	}
	return true
}

func parseMessage(src string, path_mappings []pathMapping) (metrics []metric, labels *labelset, err error) {
	metrics = make([]metric, 0)
	labels = &labelset{
		Names:  make([]string, 0),
		Values: make([]string, 0),
	}

	var s scanner.Scanner
	s.Init(strings.NewReader(src))
	var tok rune
	for {
		tok = s.Scan()
		if tok == scanner.EOF {
			return
		} else if tok != scanner.Ident {
			err = fmt.Errorf("Ident expected at %v, got %s", s.Pos(), scanner.TokenString(tok))
			return
		}
		name := s.TokenText()

		tok = s.Scan()
		if tok == ':' {
			// Metric
			tok = s.Scan()
			if tok == scanner.Float || tok == scanner.Int {
				var value float64
				value, err = strconv.ParseFloat(s.TokenText(), 64)
				if err != nil {
					return
				}
				if name == "time" {
					// varnishncsa's unit here is microseconds
					value = value / 1000000.0
				}
				metrics = append(metrics, metric{
					Name:  name,
					Value: value,
				})
			} else {
				err = fmt.Errorf("Float or Int expected at %v, got %s", s.Pos(), scanner.TokenString(tok))
				return
			}

		} else if tok == '=' {
			// Label
			tok = s.Scan()
			var value string
			if tok == scanner.Ident || tok == scanner.Float || tok == scanner.Int {
				value = s.TokenText()
			} else if tok == scanner.String {
				value, err = strconv.Unquote(s.TokenText())
				if err != nil {
					return
				}
				// a bit nasty to hardcode this, but we do hardcode the field name when running varnishncsa..
				if name == "path" {
					for i := range path_mappings {
						mapping := path_mappings[i]
						log.Debugf("replacing '%v' with '%s' in '%s'\n", mapping.Pattern, mapping.Replacement, value)
						value = mapping.Pattern.ReplaceAllString(value, mapping.Replacement)
					}
				}
			} else {
				err = fmt.Errorf("Ident or String expected at %v, got %s", s.Pos(), scanner.TokenString(tok))
			}

			labels.Names = append(labels.Names, name)
			labels.Values = append(labels.Values, value)
		} else {
			err = fmt.Errorf(": or = expected at %v, got %s", s.Pos(), scanner.TokenString(tok))
			return
		}
	}
	return
}
