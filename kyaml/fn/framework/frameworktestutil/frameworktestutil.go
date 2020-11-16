// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// Package frameworktestutil contains utilities for testing functions written using the framework.
package frameworktestutil

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// ResultsChecker tests a function by running it with predefined inputs and comparing
// the outputs to expected results.
type ResultsChecker struct {
	// TestDataDirectory is the directory containing the testdata subdirectories.
	// ResultsChecker will recurse into each test directory and run the Command
	// if the directory contains both the ConfigInputFilename and at least one
	// of ExpectedOutputFilname or ExpectedErrorFilename.
	// Defaults to "testdata"
	TestDataDirectory string

	// ConfigInputFilename is the name of the config file provided as the first
	// argument to the function.  Directories without this file will be skipped.
	// Defaults to "config.yaml"
	ConfigInputFilename string

	// InputFilenameGlob matches function inputs
	// Defaults to "input*.yaml"
	InputFilenameGlob string

	// ExpectedOutputFilname is the file with the expected output of the function
	// Defaults to "expected.yaml".  Directories containing neither this file
	// nore ExpectedErrorFilename will be skipped.
	ExpectedOutputFilname string

	// ExpectedErrorFilename is the file containing part of an expected error message
	// Defaults to "error.yaml".  Directories containing neither this file
	// nore ExpectedOutputFilname will be skipped.
	ExpectedErrorFilename string

	// Command provides the function to run.
	Command func() cobra.Command
}

// Assert asserts the results for functions
func (rc ResultsChecker) Assert(t *testing.T) bool {
	if rc.TestDataDirectory == "" {
		rc.TestDataDirectory = "testdata"
	}
	if rc.ConfigInputFilename == "" {
		rc.ConfigInputFilename = "config.yaml"
	}
	if rc.ExpectedOutputFilname == "" {
		rc.ExpectedOutputFilname = "expected.yaml"
	}
	if rc.ExpectedErrorFilename == "" {
		rc.ExpectedErrorFilename = "error.yaml"
	}
	if rc.InputFilenameGlob == "" {
		rc.InputFilenameGlob = "input*.yaml"
	}

	_ = filepath.Walk(rc.TestDataDirectory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.FailNow()
		}
		if !info.IsDir() {
			// skip non-directories
			return nil
		}
		rc.compare(t, path)
		return nil
	})

	return true
}

func (rc ResultsChecker) compare(t *testing.T, path string) {
	// make sure this directory contains test data
	configPath := filepath.Join(path, rc.ConfigInputFilename)
	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		// missing input
		return
	}
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	args := []string{configPath}

	if rc.InputFilenameGlob != "" {
		inputs, err := filepath.Glob(filepath.Join(path, rc.InputFilenameGlob))
		if !assert.NoError(t, err) {
			t.FailNow()
		}
		args = append(args, inputs...)
	}

	var actualOutput, actualError bytes.Buffer
	cmd := rc.Command()
	cmd.SetArgs(args)
	cmd.SetOut(&actualOutput)
	cmd.SetErr(&actualError)

	expectedOutput, expectedError := rc.getExpected(t, path)
	if expectedError == "" && expectedOutput == "" {
		// missing expected
		return
	}

	err = cmd.Execute()

	// Compae the results
	if expectedError != "" && !assert.Error(t, err, actualOutput.String()) {
		t.FailNow()
	}
	if expectedError == "" && !assert.NoError(t, err, actualError.String()) {
		t.FailNow()
	}
	if !assert.Equal(t,
		strings.TrimSpace(expectedOutput),
		strings.TrimSpace(actualOutput.String()), actualError.String()) {
		t.FailNow()
	}
	if !assert.Contains(t,
		strings.TrimSpace(actualError.String()),
		strings.TrimSpace(expectedError), actualOutput.String()) {
		t.FailNow()
	}
}

// getExpected reads the expected results and error files
func (rc ResultsChecker) getExpected(t *testing.T, path string) (string, string) {
	// read the expected results
	var expectedOutput, expectedError string
	if rc.ExpectedOutputFilname != "" {
		_, err := os.Stat(filepath.Join(path, rc.ExpectedOutputFilname))
		if !os.IsNotExist(err) && err != nil {
			t.FailNow()
		}
		if err == nil {
			// only read the file if it exists
			b, err := ioutil.ReadFile(filepath.Join(path, rc.ExpectedOutputFilname))
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			expectedOutput = string(b)
		}
	}
	if rc.ExpectedErrorFilename != "" {
		_, err := os.Stat(filepath.Join(path, rc.ExpectedErrorFilename))
		if !os.IsNotExist(err) && err != nil {
			t.FailNow()
		}
		if err == nil {
			// only read the file if it exists
			b, err := ioutil.ReadFile(filepath.Join(path, rc.ExpectedErrorFilename))
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			expectedError = string(b)
		}
	}
	return expectedOutput, expectedError
}
