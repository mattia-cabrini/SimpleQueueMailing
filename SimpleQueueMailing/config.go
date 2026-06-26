// Copyright (c) 2026 Mattia Cabrini
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

const EXT = "noeml"

type Config struct {
	Sender     string `yaml:"Sender"`
	SenderName string `yaml:"SenderName"`
	ReplyTo    string `yaml:"ReplyTo"`

	SmtpServer string `yaml:"SmtpServer"`
	SmtpPort   int    `yaml:"SmtpPort"`
	Password   string `yaml:"Password"`

	QueueIn       string `yaml:"QueueIn"`
	QueueOut      string `yaml:"QueueOut"`
	QueueRejected string `yaml:"QueueRejected"`

	// AuthorizedRecipients is the path to a file listing one authorized
	// recipient per line. If empty (undefined) or pointing to an empty file,
	// every recipient is authorized; otherwise only the listed ones are.
	AuthorizedRecipients string `yaml:"AuthorizedRecipients"`

	// Administrator, when set, receives a notification e-mail for every
	// rejected message.
	Administrator string `yaml:"Administrator"`

	// authorizedRecipients is the set loaded once from AuthorizedRecipients at
	// config load time. A nil/empty set means every recipient is authorized.
	authorizedRecipients map[string]bool
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

	if fi, err = os.Stat(c.QueueRejected); err != nil {
		return
	}

	if !fi.IsDir() {
		return errors.New("QueueRejected is not a directory")
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

	fmt.Print(helper)
	os.Exit(0)
}

func printSampleConfig() {
	if len(os.Args) < 2 {
		return
	}

	if os.Args[1] != "config" {
		return
	}

	fmt.Print(sampleConfig)
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

	if conf.AuthorizedRecipients != "" {
		conf.authorizedRecipients, err = loadAuthorizedRecipients(conf.AuthorizedRecipients)
		utility.Mypanic(err)
	}

	return
}
