// Copyright (c) 2023 Mattia Cabrini
// SPDX-License-Identifier: MIT

package SimpleQueueMailing

import (
	"bufio"
	_ "embed"
	"errors"
	"fmt"
	"github.com/mattia-cabrini/go-utility"
	"io"
	"os"
	"strings"
	"time"
)

type message struct {
	Headers []string
	Content []byte
}

func InitMessageFromFile(conf *Config, path string) (m message, err error) {
	fp, err := os.OpenFile(path, os.O_RDONLY, 0400)

	if err != nil {
		return
	}

	defer utility.Deferrable(fp.Close, nil, nil)

	var k = bufio.NewScanner(fp)

	for k.Scan() {
		var line string

		if line = k.Text(); len(line) == 0 {
			break
		}

		m.Headers = append(m.Headers, line)
	}

	if err = k.Err(); err != nil {
		return
	}

	if _, err = fp.Seek(0, 0); err != nil {
		return
	}

	m.Content, err = io.ReadAll(fp)

	if len(m.Content) <= 4 {
		err = errors.New("file too short")
		return
	}

	var target = [4]byte{13, 10, 13, 10}
	var temp [4]byte

	for i := 3; i < len(m.Content); i++ {
		for ix, bx := range m.Content[i-3 : i+1] {
			temp[ix] = bx
		}

		if temp == target {
			m.Content = m.Content[i+1:]
			break
		}
	}

	fi, err := os.Stat(path)
	if err != nil {
		return
	}

	if m.Header("Date") == "" {
		m.Headers = append(m.Headers,
			fmt.Sprintf("Date: %s", fi.ModTime().Format(time.RFC1123Z)),
		)
	}

	m.Headers = append(m.Headers, fmt.Sprintf("User-Agent: SimpleQueueMailing"))

	if conf.ReplyTo != "" {
		m.Headers = append(m.Headers, fmt.Sprintf("Reply-To: %s", conf.ReplyTo))
	}

	return
}

func (m *message) PrintTo(conf *Config, w io.Writer) (err error) {
	if conf.SenderName == "" {
		_, err = fmt.Fprintf(w, "From: %s\r\n", conf.Sender)
	} else {
		_, err = fmt.Fprintf(w, "From: %s <%s>\r\n", conf.SenderName, conf.Sender)
	}
	if err != nil {
		return
	}

	for _, hx := range m.Headers {
		_, err = fmt.Fprintf(w, "%s\r\n", hx)
		if err != nil {
			return
		}
	}

	_, err = fmt.Fprintf(w, "\r\n")
	if err != nil {
		return
	}

	// appending EML...
	_, err = w.Write(m.Content)
	return
}

func (m *message) Header(name string) (value string) {
	for _, hx := range m.Headers {
		if len(hx) < len(name) {
			continue
		}

		if hx[:len(name)] == name {
			value = hx[len(name)+1:] // considering trailing ':'
			value = strings.TrimLeft(value, " ")
			break
		}
	}

	return
}

func (m *message) Re() string {
	return m.Header("Subject")
}

func (m *message) To() (tos []string) {
	toH := m.Header("To")
	tos = strings.Split(toH, ";")
	return
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
		m, err = InitMessageFromFile(conf, path)
		found = true

		if err == nil {
			fileOut := fmt.Sprintf("%s/%d_%s", conf.QueueOut, time.Now().UnixNano(), name)
			err = os.Rename(path, fileOut)
		}
	}

	return
}
