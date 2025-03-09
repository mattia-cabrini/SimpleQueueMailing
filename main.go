// Copyright (c) 2023 Mattia Cabrini
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"crypto/tls"
	_ "embed"
	"errors"
	"fmt"
	"github.com/mattia-cabrini/go-utility"
	"gopkg.in/yaml.v3"
	"net/smtp"
	"os"
	"strings"
	"time"
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

type Message struct {
	To []string
	Re string

	Text string
}

func InitMessageFromFile(path string) (m Message, err error) {
	fp, err := os.OpenFile(path, os.O_RDONLY, 0400)

	if err != nil {
		return
	}

	defer utility.Deferrable(fp.Close, nil, nil)

	var k = bufio.NewScanner(fp)
	var intoText = false

	for k.Scan() {
		line := k.Text()

		if intoText {
			m.Text = m.Text + k.Text()
		} else {
			var tline string
			if len(line) >= 8 {
				tline = strings.Trim(line[:8], " ")
			}

			switch {
			case line == "":
				intoText = true
			case tline == "A":
				m.To = append(m.To, line[8:])
			case tline == "Re":
				m.Re = line[8:]
			}
		}
	}

	err = k.Err()

	return
}

func (m *Message) Dump() {
	for _, tox := range m.To {
		fmt.Printf("To: %s\n", tox)
	}

	fmt.Printf("\nRe: %s\n\n==========\n%s\n==========\n", m.Re, m.Text)
}

func (m *Message) Msg(conf *Config) []byte {
	var b strings.Builder

	fmt.Fprintf(&b, "From: %s\r\n", conf.Sender)

	for i, tox := range m.To {
		if i > 0 {
			fmt.Fprintf(&b, "; %s", tox)
		} else {
			fmt.Fprintf(&b, "To: %s", tox)
		}
	}
	fmt.Fprintf(&b, "\r\n")
	fmt.Fprintf(&b, "Subject: %s\r\n", m.Re)
	// fmt.Fprintf(&b, "Content-type: text/plain")

	fmt.Fprintf(&b, "\r\n%s", m.Text)
	return []byte(b.String())
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

func CreateMessageFrom(conf *Config) (m Message, found bool, err error) {
	var path string
	var name string
	entries, err := os.ReadDir(conf.QueueIn)

	if err == nil && len(entries) > 0 {
		for _, ex := range entries {
			name = ex.Name()

			if len(name) < len(EXT) {
				continue
			}

			if name[len(name)-len(EXT):] != EXT {
				continue
			}

			path = conf.QueueIn + "/" + name
			break
		}
	}

	if len(path) > 0 {
		m, err = InitMessageFromFile(path)
		found = true

		if err == nil {
			fileOut := fmt.Sprintf("%s/%d_%s", conf.QueueOut, time.Now().UnixNano(), name)
			err = os.Rename(path, fileOut)
		}
	}

	return
}

func ExecuteMailing(conf *Config) {
	m, f, err := CreateMessageFrom(conf)

	if err != nil {
		utility.Logf(utility.ERROR, err.Error())
		return
	}

	if f {
		// m.Dump()
		host := fmt.Sprintf("%s:%d", conf.SmtpServer, conf.SmtpPort)

		tlsConfig := &tls.Config{
			InsecureSkipVerify: true, // Da usare solo in ambiente di test
			ServerName:         conf.SmtpServer,
		}

		conn, err := tls.Dial("tcp", host, tlsConfig)
		if err != nil {
			utility.Logf(utility.ERROR,
				"Could not send mail test (tls dial failed) %s - %s",
				m.Re, err.Error(),
			)
			return
		}

		client, err := smtp.NewClient(conn, conf.SmtpServer)
		if err != nil {
			defer utility.Deferrable(conn.Close, nil, nil)
			utility.Logf(utility.ERROR,
				"Could not send mail (could not create new client) %s - %s",
				m.Re, err.Error(),
			)
			return
		}
		defer utility.Deferrable(client.Quit, nil, nil)

		auth := smtp.PlainAuth("", conf.Sender, conf.Password, conf.SmtpServer)
		if err = client.Auth(auth); err != nil {
			utility.Logf(utility.ERROR,
				"Could not send mail (plain auth failed) %s - %s",
				m.Re, err.Error(),
			)
			return
		}

		if err = client.Mail(conf.Sender); err != nil {
			utility.Logf(utility.ERROR,
				"Could not send mail (set sender failed) %s - %s",
				m.Re, err.Error(),
			)
			return
		}

		for _, addr := range m.To {
			if err = client.Rcpt(addr); err != nil {
				utility.Logf(utility.ERROR,
					"Could not send mail (set receipient %s failed) %s - %s",
					addr, m.Re, err.Error(),
				)
				return
			}
		}

		w, err := client.Data()
		if err != nil {
			utility.Logf(utility.ERROR,
				"Could not send mail (could not init writer) %s - %s",
				m.Re, err.Error(),
			)
			return
		}
		_, err = w.Write(m.Msg(conf))
		if err != nil {
			utility.Logf(utility.ERROR,
				"Could not send mail (could not write message) %s - %s",
				m.Re, err.Error(),
			)
			return
		}
		err = w.Close()
		if err != nil {
			utility.Logf(utility.ERROR,
				"Could not send mail (could not close writer) %s - %s",
				m.Re, err.Error(),
			)
			return
		}

		// err = smtp.SendMail(host, auth, conf.Sender, m.To, m.Msg(conf))
		// if err != nil {
		// 	utility.Logf(utility.ERROR, "Could not send mail %s - %s", m.Re, err.Error())
		// 	return
		// }
		utility.Logf(utility.WARNING, "Sent mail %s to %v", m.Re, m.To)
	}
}

func main() {
	printHelp()
	printSampleConfig()

	conf := readConfig()

	err := conf.Check()
	utility.Mypanic(err)

	for {
		ExecuteMailing(&conf)
		time.Sleep(100 * time.Millisecond)
	}
}
