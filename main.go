package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
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
	Use:   "txtplate [flags] <valuesfile>",
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
	rootCmd.Args = cobra.ExactArgs(1)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func doTemplating(cmd *cobra.Command, args []string) error {
	var byt []byte
	var err error

	byt, err = ioutil.ReadFile(args[0])
	if err != nil {
		return errors.Wrap(err, "failed to read values file")
	}

	var data interface{}

	switch filepath.Ext(args[0]) {
	case ".yaml", ".yml":
		if err = yaml.Unmarshal(byt, &data); err != nil {
			return errors.Wrap(err, "failed to parse values file as yaml")
		}
	default:
		if err = json.Unmarshal(byt, &data); err != nil {
			return errors.Wrap(err, "failed to parse values file as json")
		}
	}

	if len(flagInput) != 0 {
		byt, err = ioutil.ReadFile(flagInput)
	} else {
		byt, err = ioutil.ReadAll(os.Stdin)
	}

	if err != nil {
		return errors.Wrap(err, "failed to read input")
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
