// Copyright © 2016 Wei Shen <shenwei356@gmail.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cmd

import (
	"encoding/csv"
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/shenwei356/util/stringutil"
	"github.com/shenwei356/xopen"
	"github.com/spf13/cobra"
)

// freqCmd represents the freq command
var freqCmd = &cobra.Command{
	Use:   "freq",
	Short: "frequencies of selected fields",
	Long: `frequencies of selected fields

`,
	Run: func(cmd *cobra.Command, args []string) {
		config := getConfigs(cmd)
		files := getFileList(args)
		if len(files) > 1 {
			checkError(fmt.Errorf("no more than one file should be given"))
		}
		runtime.GOMAXPROCS(config.NumCPUs)

		sortByFreq := getFlagBool(cmd, "sort-by-freq")
		sortByKey := getFlagBool(cmd, "sort-by-key")
		reverse := getFlagBool(cmd, "reverse")

		fieldStr := getFlagString(cmd, "fields")
		if fieldStr == "" {
			checkError(fmt.Errorf("flag -f (--fields) needed"))
		}
		fields, colnames, negativeFields, needParseHeaderRow := parseFields(cmd, fieldStr, config.NoHeaderRow)
		var fieldsMap map[int]struct{}
		if len(fields) > 0 {
			fields2 := make([]int, len(fields))
			fieldsMap = make(map[int]struct{}, len(fields))
			for i, f := range fields {
				if negativeFields {
					fieldsMap[f*-1] = struct{}{}
					fields2[i] = f * -1
				} else {
					fieldsMap[f] = struct{}{}
					fields2[i] = f
				}
			}
			fields = fields2
		}

		fuzzyFields := getFlagBool(cmd, "fuzzy-fields")

		outfh, err := xopen.Wopen(config.OutFile)
		checkError(err)
		defer outfh.Close()

		writer := csv.NewWriter(outfh)
		if config.OutTabs || config.Tabs {
			writer.Comma = '\t'
		} else {
			writer.Comma = config.OutDelimiter
		}

		counter := make(map[string]int, 10000)

		file := files[0]
		csvReader, err := newCSVReaderByConfig(config, file)
		checkError(err)
		csvReader.Run()

		parseHeaderRow := needParseHeaderRow // parsing header row
		printHeaderRow := needParseHeaderRow
		var colnames2fileds map[string]int // column name -> field
		var colnamesMap map[string]*regexp.Regexp

		checkFields := true
		var items []string
		var key string

		printMetaLine := true
		for chunk := range csvReader.Ch {
			checkError(chunk.Err)

			if printMetaLine && len(csvReader.Reader.MetaLine) > 0 {
				outfh.WriteString(fmt.Sprintf("sep=%s\n", string(writer.Comma)))
				printMetaLine = false
			}

			for _, record := range chunk.Data {
				if parseHeaderRow { // parsing header row
					colnames2fileds = make(map[string]int, len(record))
					for i, col := range record {
						colnames2fileds[col] = i + 1
					}
					colnamesMap = make(map[string]*regexp.Regexp, len(colnames))
					for _, col := range colnames {
						if !fuzzyFields {
							if _, ok := colnames2fileds[col]; !ok {
								checkError(fmt.Errorf(`column "%s" not existed in file: %s`, col, file))
							}
						}
						if negativeFields {
							colnamesMap[col[1:]] = fuzzyField2Regexp(col[1:])
						} else {
							colnamesMap[col] = fuzzyField2Regexp(col)
						}
					}

					if len(fields) == 0 { // user gives the colnames
						fields = []int{}
						for _, col := range record {
							var ok bool
							if fuzzyFields {
								for _, re := range colnamesMap {
									if re.MatchString(col) {
										ok = true
										break
									}
								}
							} else {
								_, ok = colnamesMap[col]
							}
							if ok {
								fields = append(fields, colnames2fileds[col])
							}
						}
					}

					fieldsMap = make(map[int]struct{}, len(fields))
					for _, f := range fields {
						fieldsMap[f] = struct{}{}
					}

					parseHeaderRow = false
				}
				if checkFields {
					for field := range fieldsMap {
						if field > len(record) {
							checkError(fmt.Errorf(`field (%d) out of range (%d) in file: %s`, field, len(record), file))
						}
					}
					fields2 := []int{}
					for f := range record {
						_, ok := fieldsMap[f+1]
						if negativeFields {
							if !ok {
								fields2 = append(fields2, f+1)
							}
						} else {
							if ok {
								fields2 = append(fields2, f+1)
							}
						}
					}
					fields = fields2
					if len(fields) == 0 {
						checkError(fmt.Errorf("no fields matched in file: %s", file))
					}
					items = make([]string, len(fields))

					checkFields = false
				}

				for i, f := range fields {
					items[i] = record[f-1]
				}

				if printHeaderRow {
					checkError(writer.Write(append(items, "frequency")))
					printHeaderRow = false
					continue
				}

				key = strings.Join(items, "_shenwei356_")
				counter[key]++
			}
		}

		if sortByFreq {
			counts := make([]stringutil.StringCount, len(counter))
			i := 0
			for key, count := range counter {
				counts[i] = stringutil.StringCount{Key: key, Count: count}
				i++
			}
			if reverse {
				sort.Sort(stringutil.ReversedStringCountList{counts})
			} else {
				sort.Sort(stringutil.StringCountList(counts))
			}
			for _, count := range counts {
				items = strings.Split(count.Key, "_shenwei356_")
				items = append(items, strconv.Itoa(counter[count.Key]))
				checkError(writer.Write(items))
			}
		} else if sortByKey {
			keys := make([]string, len(counter))
			i := 0
			for key := range counter {
				keys[i] = key
				i++
			}

			sort.Strings(keys)
			if reverse {
				stringutil.ReverseStringSliceInplace(keys)
			}

			for _, key := range keys {
				items = strings.Split(key, "_shenwei356_")
				items = append(items, strconv.Itoa(counter[key]))
				checkError(writer.Write(items))
			}
		} else {
			for key, count := range counter {
				items = strings.Split(key, "_shenwei356_")
				items = append(items, strconv.Itoa(count))
				checkError(writer.Write(items))
			}
		}

		writer.Flush()
		checkError(writer.Error())
	},
}

func init() {
	RootCmd.AddCommand(freqCmd)
	freqCmd.Flags().StringP("fields", "f", "1", `select only these fields. e.g -f 1,2 or -f columnA,columnB`)
	freqCmd.Flags().BoolP("ignore-case", "i", false, `ignore case`)
	freqCmd.Flags().BoolP("fuzzy-fields", "F", false, `using fuzzy fileds, e.g. *name or id123*`)
	freqCmd.Flags().BoolP("sort-by-freq", "n", false, `sort by frequency`)
	freqCmd.Flags().BoolP("sort-by-key", "k", false, `sort by key`)
	freqCmd.Flags().BoolP("reverse", "r", false, `reverse order while sorting`)
}
