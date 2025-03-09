// Copyright (c) 2023 Mattia Cabrini
// SPDX-License-Identifier: MIT

package SimpleQueueMailing

import (
	"bufio"
	_ "embed"
	"fmt"
	"github.com/mattia-cabrini/go-utility"
	"os"
	"strings"
	"time"
)

type message struct {
	To []string
	Re string

	Text string
}

func InitMessageFromFile(path string) (m message, err error) {
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

func (m *message) Dump() {
	for _, tox := range m.To {
		fmt.Printf("To: %s\n", tox)
	}

	fmt.Printf("\nRe: %s\n\n==========\n%s\n==========\n", m.Re, m.Text)
}

func (m *message) Msg(conf *Config) []byte {
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

func CreateMessageFrom(conf *Config) (m message, found bool, err error) {
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

