// Copyright (c) 2023 Mattia Cabrini
// SPDX-License-Identifier: MIT

package SimpleQueueMailing

import (
	_ "embed"
	"errors"
	"fmt"
	"github.com/mattia-cabrini/go-utility"
	"gopkg.in/yaml.v3"
	"os"
)

//go:embed helper.txt
var helper string

//go:embed sample_config.yaml
var sampleConfig string

const EXT = "nohtml"

type Config struct {
	Sender string `yaml:"Sender"`

	SmtpServer string `yaml:"SmtpServer"`
	SmtpPort   int    `yaml:"SmtpPort"`
	Password   string `yaml:"Password"`

	QueueIn  string `yaml:"QueueIn"`
	QueueOut string `yaml:"QueueOut"`
}

func (c *Config) Check() (err error) {
	var fi os.FileInfo

	if fi, err = os.Stat(c.QueueIn); err != nil {
		return
	}

	if !fi.IsDir() {
		return errors.New("QueueIn is not a directory")
	}

	if fi, err = os.Stat(c.QueueOut); err != nil {
		return
	}

	if !fi.IsDir() {
		return errors.New("QueueOut is not a directory")
	}

	return
}

func printHelp() {
	if len(os.Args) < 2 {
		return
	}

	if os.Args[1] != "help" {
		return
	}

	fmt.Printf(helper)
	os.Exit(0)
}

func printSampleConfig() {
	if len(os.Args) < 2 {
		return
	}

	if os.Args[1] != "config" {
		return
	}

	fmt.Printf(sampleConfig)
	os.Exit(0)
}

func readConfig() (conf Config) {
	if len(os.Args) != 2 {
		utility.Logf(utility.FATAL, "wrong arguments")
	}

	fp, err := os.ReadFile(os.Args[1])
	utility.Mypanic(err)

	err = yaml.Unmarshal(fp, &conf)
	utility.Mypanic(err)

	return
}

