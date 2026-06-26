// Copyright (c) 2026 Mattia Cabrini
// SPDX-License-Identifier: MIT

package SimpleQueueMailing

import (
	"bufio"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattia-cabrini/go-utility"
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
		copy(temp[:], m.Content[i-3:i+1])

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

	m.Headers = append(m.Headers, "User-Agent: SimpleQueueMailing")

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
	prefix := name + ":" // a header line must be "<name>:<value>"

	for _, hx := range m.Headers {
		if len(hx) < len(prefix) {
			continue
		}

		if strings.EqualFold(hx[:len(prefix)], prefix) {
			value = strings.TrimLeft(hx[len(prefix):], " ")
			break
		}
	}

	return
}

func (m *message) Re() string {
	return m.Header("Subject")
}

// recipientsAuthorized reports whether every recipient of the message is
// allowed to receive it. When the authorized set is empty (AuthorizedRecipients
// undefined, or an empty file) every recipient is authorized; otherwise a
// recipient is allowed only if it appears (case-insensitively) in the set.
func (m *message) recipientsAuthorized(conf *Config) bool {
	if len(conf.authorizedRecipients) == 0 {
		return true
	}

	for _, addr := range m.To() {
		if !conf.authorizedRecipients[strings.ToLower(strings.TrimSpace(addr))] {
			return false
		}
	}

	return true
}

// loadAuthorizedRecipients reads the authorized recipients file into a set of
// lower-cased addresses, ignoring blank lines.
func loadAuthorizedRecipients(path string) (allowed map[string]bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	allowed = make(map[string]bool)

	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			allowed[strings.ToLower(line)] = true
		}
	}

	return
}

func (m *message) To() (tos []string) {
	// Header is case-insensitive, so "To"/"Cc" already cover any casing.
	if ToH := strings.TrimSpace(m.Header("To")); ToH != "" {
		tos = append(tos, strings.Split(ToH, ",")...)
	}

	if CcH := strings.TrimSpace(m.Header("Cc")); CcH != "" {
		tos = append(tos, strings.Split(CcH, ",")...)
	}

	return
}

func CreateMessageFrom(conf *Config) (m message, found bool, name string, path string, err error) {
	entries, err := os.ReadDir(conf.QueueIn)
	if err != nil {
		return
	}

	for _, ex := range entries {
		nx := ex.Name()

		if len(nx) < len(EXT) {
			continue
		}

		if nx[len(nx)-len(EXT):] != EXT {
			continue
		}

		name = nx
		path = conf.QueueIn + "/" + nx
		break
	}

	if len(path) > 0 {
		found = true
		m, err = InitMessageFromFile(conf, path)
	}

	return
}

// MoveToQueue moves the file at path into targetDir, keeping the original name
// prefixed with a timestamp so that names do not collide.
func MoveToQueue(targetDir, name, path string) error {
	fileOut := fmt.Sprintf("%s/%d_%s", targetDir, time.Now().UnixNano(), name)
	return os.Rename(path, fileOut)
}

// fileSHA256 returns the hex-encoded SHA-256 of the file at path.
func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
