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
	"time"
)

//go:embed helper.txt
var helper string

//go:embed sample_config.yaml
var sampleConfig string

const EXT = "noeml"

// Defaults, in milliseconds, for the pauses missing from the config file.
const (
	defaultSendPause           = 6000    // 6 seconds
	defaultServerFaultPauseMin = 1000    // 1 second
	defaultServerFaultPauseMax = 3600000 // 1 hour
)

type Config struct {
	Sender     string `yaml:"Sender"`
	SenderName string `yaml:"SenderName"`
	ReplyTo    string `yaml:"ReplyTo"`

	SmtpServer string `yaml:"SmtpServer"`
	SmtpPort   int    `yaml:"SmtpPort"`
	Password   string `yaml:"Password"`

	// SmtpInsecureSkipVerify disables the verification of the SMTP server TLS
	// certificate. It exposes the connection, and the password sent with PLAIN
	// auth, to man-in-the-middle attacks: enable it only for testing.
	SmtpInsecureSkipVerify bool `yaml:"SmtpInsecureSkipVerify"`

	// SendPause is the pause after every delivery attempt, in milliseconds; it
	// caps the sending rate. It defaults to defaultSendPause when undefined,
	// and 0 disables it.
	SendPause int `yaml:"SendPause"`

	// ServerFaultPauseMin and ServerFaultPauseMax bound the pause, in
	// milliseconds, before a message is retried after an SMTP server fault
	// (see errServerFault and serverFaultBackoff): the first pause is
	// ServerFaultPauseMin, every further consecutive fault doubles it, up to
	// ServerFaultPauseMax. They default to defaultServerFaultPauseMin and
	// defaultServerFaultPauseMax when undefined; a minimum of 0 disables the
	// pause.
	ServerFaultPauseMin int `yaml:"ServerFaultPauseMin"`
	ServerFaultPauseMax int `yaml:"ServerFaultPauseMax"`

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

	if c.SendPause < 0 {
		return errors.New("SendPause is negative")
	}

	if c.ServerFaultPauseMin < 0 {
		return errors.New("ServerFaultPauseMin is negative")
	}

	if c.ServerFaultPauseMin > c.ServerFaultPauseMax {
		return errors.New("ServerFaultPauseMin is greater than ServerFaultPauseMax")
	}

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

// sendPause returns SendPause as a time.Duration.
func (c *Config) sendPause() time.Duration {
	return time.Duration(c.SendPause) * time.Millisecond
}

// serverFaultPauseMin returns ServerFaultPauseMin as a time.Duration.
func (c *Config) serverFaultPauseMin() time.Duration {
	return time.Duration(c.ServerFaultPauseMin) * time.Millisecond
}

// serverFaultPauseMax returns ServerFaultPauseMax as a time.Duration.
func (c *Config) serverFaultPauseMax() time.Duration {
	return time.Duration(c.ServerFaultPauseMax) * time.Millisecond
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
		logf(utility.FATAL, "wrong arguments")
	}

	fp, err := os.ReadFile(os.Args[1])
	fatalIf(err, "Could not read config file "+os.Args[1])

	// Defaults: yaml.Unmarshal leaves fields missing from the file untouched.
	conf.SendPause = defaultSendPause
	conf.ServerFaultPauseMin = defaultServerFaultPauseMin
	conf.ServerFaultPauseMax = defaultServerFaultPauseMax

	err = yaml.Unmarshal(fp, &conf)
	fatalIf(err, "Could not parse config file "+os.Args[1])

	if conf.AuthorizedRecipients != "" {
		conf.authorizedRecipients, err = loadAuthorizedRecipients(conf.AuthorizedRecipients)
		fatalIf(err, "Could not load authorized recipients from "+conf.AuthorizedRecipients)
	}

	return
}
