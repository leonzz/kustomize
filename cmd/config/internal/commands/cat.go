// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"sigs.k8s.io/kustomize/cmd/config/ext"
	"sigs.k8s.io/kustomize/cmd/config/internal/generateddocs/commands"
	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// GetCatRunner returns a command CatRunner.
func GetCatRunner(name string) *CatRunner {
	r := &CatRunner{}
	c := &cobra.Command{
		Use:     "cat DIR",
		Short:   commands.CatShort,
		Long:    commands.CatLong,
		Example: commands.CatExamples,
		RunE:    r.runE,
		Args:    cobra.MaximumNArgs(1),
	}
	fixDocs(name, c)
	c.Flags().BoolVar(&r.Format, "format", true,
		"format resource config yaml before printing.")
	c.Flags().BoolVar(&r.KeepAnnotations, "annotate", false,
		"annotate resources with their file origins.")
	c.Flags().StringVar(&r.WrapKind, "wrap-kind", "",
		"if set, wrap the output in this list type kind.")
	c.Flags().StringVar(&r.WrapApiVersion, "wrap-version", "",
		"if set, wrap the output in this list type apiVersion.")
	c.Flags().StringVar(&r.FunctionConfig, "function-config", "",
		"path to function config to put in ResourceList -- only if wrapped in a ResourceList.")
	c.Flags().StringSliceVar(&r.Styles, "style", []string{},
		"yaml styles to apply.  may be 'TaggedStyle', 'DoubleQuotedStyle', 'LiteralStyle', "+
			"'FoldedStyle', 'FlowStyle'.")
	c.Flags().BoolVar(&r.StripComments, "strip-comments", false,
		"remove comments from yaml.")
	c.Flags().BoolVar(&r.IncludeLocal, "include-local", false,
		"if true, include local-config in the output.")
	c.Flags().BoolVar(&r.ExcludeNonLocal, "exclude-non-local", false,
		"if true, exclude non-local-config in the output.")
	c.Flags().StringVar(&r.OutputDest, "dest", "",
		"if specified, write output to a file rather than stdout")
	c.Flags().BoolVarP(&r.RecurseSubPackages, "recurse-subpackages", "R", true,
		"print resources recursively in all the nested subpackages")
	r.Command = c
	return r
}

func CatCommand(name string) *cobra.Command {
	return GetCatRunner(name).Command
}

// CatRunner contains the run function
type CatRunner struct {
	Format             bool
	KeepAnnotations    bool
	WrapKind           string
	WrapApiVersion     string
	FunctionConfig     string
	OutputDest         string
	Styles             []string
	StripComments      bool
	IncludeLocal       bool
	ExcludeNonLocal    bool
	Command            *cobra.Command
	RecurseSubPackages bool
}

func (r *CatRunner) runE(c *cobra.Command, args []string) error {
	var writer = c.OutOrStdout()
	if r.OutputDest != "" {
		o, err := os.Create(r.OutputDest)
		if err != nil {
			return errors.Wrap(err)
		}
		defer o.Close()
		writer = o
	}
	if len(args) == 0 {
		input := &kio.ByteReader{Reader: c.InOrStdin()}
		// if there is a function-config specified, emit it
		outputs, err := r.out(writer)
		if err != nil {
			return err
		}
		return handleError(c, kio.Pipeline{Inputs: []kio.Reader{input}, Filters: r.catFilters(), Outputs: outputs}.Execute())
	}

	e := executeCmdOnPkgs{
		writer:             writer,
		needOpenAPI:        false,
		recurseSubPackages: r.RecurseSubPackages,
		cmdRunner:          r,
		rootPkgPath:        args[0],
		skipPkgPathPrint:   true,
	}

	return e.execute()
}

func (r *CatRunner) executeCmd(w io.Writer, pkgPath string) error {
	openAPIFileName, err := ext.OpenAPIFileName()
	if err != nil {
		return err
	}

	input := kio.LocalPackageReader{PackagePath: pkgPath, PackageFileName: openAPIFileName}
	outputs, err := r.out(w)
	if err != nil {
		return err
	}
	err = kio.Pipeline{
		Inputs:  []kio.Reader{input},
		Filters: r.catFilters(),
		Outputs: outputs,
	}.Execute()

	if err != nil {
		// return err if there is only package
		if !r.RecurseSubPackages {
			return err
		} else {
			// print error message and continue if there are multiple packages to annotate
			fmt.Fprintf(w, "%s in package %q\n", err.Error(), pkgPath)
		}
	}
	fmt.Fprintf(w, "---")
	return nil
}

func (r *CatRunner) catFilters() []kio.Filter {
	var fltrs []kio.Filter
	// don't include reconcilers
	fltrs = append(fltrs, &filters.IsLocalConfig{
		IncludeLocalConfig:    r.IncludeLocal,
		ExcludeNonLocalConfig: r.ExcludeNonLocal,
	})
	if r.Format {
		fltrs = append(fltrs, filters.FormatFilter{})
	}
	if r.StripComments {
		fltrs = append(fltrs, filters.StripCommentsFilter{})
	}
	return fltrs
}

func (r *CatRunner) out(w io.Writer) ([]kio.Writer, error) {
	var outputs []kio.Writer
	var functionConfig *yaml.RNode
	if r.FunctionConfig != "" {
		configs, err := kio.LocalPackageReader{PackagePath: r.FunctionConfig,
			OmitReaderAnnotations: !r.KeepAnnotations}.Read()
		if err != nil {
			return outputs, err
		}
		if len(configs) != 1 {
			return outputs, fmt.Errorf("expected exactly 1 functionConfig, found %d", len(configs))
		}
		functionConfig = configs[0]
	}

	// remove this annotation explicitly, the ByteWriter won't clear it by
	// default because it doesn't set it
	clear := []string{"config.kubernetes.io/path"}
	if r.KeepAnnotations {
		clear = nil
	}

	outputs = append(outputs, kio.ByteWriter{
		Writer:                w,
		KeepReaderAnnotations: r.KeepAnnotations,
		WrappingKind:          r.WrapKind,
		WrappingAPIVersion:    r.WrapApiVersion,
		FunctionConfig:        functionConfig,
		Style:                 yaml.GetStyle(r.Styles...),
		ClearAnnotations:      clear,
	})

	return outputs, nil
}
