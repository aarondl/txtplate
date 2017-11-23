package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"text/template"

	yaml "gopkg.in/yaml.v2"

	"github.com/Masterminds/sprig"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	flagInput  string
	flagOutput string
)

var rootCmd = cobra.Command{
	Use:   "txtplate [flags] <valuesfiles...>",
	Short: "Apply values in a json or yaml file to go text/templated templates",
	Long: `By default run stdin (or --input) through the go templating engine
and output the result to stdout (or --output). Template functions available
are from the sprig (https://github.com/Masterminds/sprig) package. Detects
the file type of valuesfile based on extension, defaults to json if omitted.

Example:
	cat mytemplate.tpl | txtplate values.json > output.txt
`,
	RunE: doTemplating,
}

func main() {
	flags := rootCmd.Flags()
	flags.StringVarP(&flagInput, "input", "i", "", "Input from the file given instead of stdin")
	flags.StringVarP(&flagOutput, "output", "o", "", "Output from the file given instead of stdout")
	rootCmd.Args = cobra.MinimumNArgs(1)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func doTemplating(cmd *cobra.Command, args []string) error {
	var byt []byte
	var err error

	if len(flagInput) != 0 {
		byt, err = ioutil.ReadFile(flagInput)
	} else {
		byt, err = ioutil.ReadAll(os.Stdin)
	}
	if err != nil {
		return errors.Wrap(err, "failed to read input")
	}

	data, err := readValuesFiles(args)
	if err != nil {
		return err
	}

	tpl, err := template.New("").Funcs(sprig.TxtFuncMap()).Parse(string(byt))
	if err != nil {
		return errors.Wrap(err, "failed to compile template")
	}

	output := &bytes.Buffer{}
	if err = tpl.Execute(output, data); err != nil {
		return errors.Wrap(err, "failed to execute template")
	}

	if len(flagOutput) != 0 {
		err = ioutil.WriteFile(flagOutput, output.Bytes(), 0664)
	} else {
		_, err = io.Copy(os.Stdout, output)
	}

	if err != nil {
		return errors.Wrap(err, "failed to write output")
	}

	return nil
}

func readValuesFiles(files []string) (interface{}, error) {
	data := map[string]interface{}{}

	for _, file := range files {
		byt, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read values file")
		}

		var incomingData map[string]interface{}
		switch filepath.Ext(file) {
		case ".yaml", ".yml":
			if err = yaml.Unmarshal(byt, &incomingData); err != nil {
				return nil, errors.Wrapf(err, "failed to parse values file %s as yaml", file)
			}
			incomingData = convertToMapStringIntf(incomingData).(map[string]interface{})
		default:
			if err = json.Unmarshal(byt, &incomingData); err != nil {
				return nil, errors.Wrapf(err, "failed to parse values file %s as json", file)
			}
		}

		data, err = mergeMaps(data, incomingData)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}

// convertToMapStringIntf takes a object and recursively attempts to
// convert any maps in it of type map[interface{}]interface{} to
// map[string]interface{}, all other values are simply returned.
func convertToMapStringIntf(value interface{}) interface{} {
	switch m := value.(type) {
	case map[string]interface{}:
		for k, v := range m {
			m[k] = convertToMapStringIntf(v)
		}
		return m
	case map[interface{}]interface{}:
		newMap := make(map[string]interface{}, len(m))
		for k, v := range m {
			newMap[k.(string)] = convertToMapStringIntf(v)
		}
		return newMap
	default:
		return value
	}
}

var strMapType = reflect.TypeOf(map[string]interface{}{})

// mergeMaps takes two map[string]interface{}
// and attempts to merge them into dst. Keys that exist
// in dst will be overwritten with values from src.
func mergeMaps(dst, src interface{}) (map[string]interface{}, error) {
	m, err := mergeMapsHelper(reflect.ValueOf(dst), reflect.ValueOf(src))
	if err != nil {
		return nil, err
	}

	return m.(map[string]interface{}), nil
}

func mergeMapsHelper(dst, src reflect.Value) (interface{}, error) {
	if dst.Type() != strMapType {
		return nil, errors.New("dst was not a map[string]interface{}")
	}
	if src.Type() != strMapType {
		return nil, errors.New("src was not a map[string]interface{}")
	}

	for _, key := range src.MapKeys() {
		srcValue := src.MapIndex(key).Elem()
		dstValue := dst.MapIndex(key)
		srcType := srcValue.Type()
		var dstType reflect.Type

		if dstValue.IsValid() {
			dstValue = dstValue.Elem()
			dstType = dstValue.Type()

			if srcType == strMapType && dstType == strMapType {
				intf, err := mergeMapsHelper(dstValue, srcValue)
				if err != nil {
					return nil, err
				}

				dst.SetMapIndex(key, reflect.ValueOf(intf))
				continue
			}
		}

		dst.SetMapIndex(key, srcValue)
	}

	return dst.Interface(), nil
}
