// Copyright (c) 2026 Mattia Cabrini
// SPDX-License-Identifier: MIT

package SimpleQueueMailing

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/mattia-cabrini/go-utility"
)

type message struct {
	Headers []string
	Content []byte
}

func InitMessageFromFile(conf *Config, path string) (m message, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	if len(data) <= 4 {
		err = errors.New("file too short")
		return
	}

	// Headers and body are separated by a blank line; everything before it is
	// the header block, everything after it is the content.
	headerBlock, content, _ := bytes.Cut(data, []byte("\r\n\r\n"))
	m.Content = content

	for line := range strings.SplitSeq(string(headerBlock), "\r\n") {
		if line == "" {
			break
		}
		m.Headers = append(m.Headers, line)
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

// recipientsAuthorized reports whether every recipient in rcpts (as returned by
// message.To) is allowed to receive the message. When the authorized set is
// empty (AuthorizedRecipients undefined, or an empty file) every recipient is
// authorized; otherwise a recipient is allowed only if it appears
// (case-insensitively) in the set.
func recipientsAuthorized(conf *Config, rcpts []string) bool {
	if len(conf.authorizedRecipients) == 0 {
		return true
	}

	for _, addr := range rcpts {
		if !conf.authorizedRecipients[strings.ToLower(addr)] {
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

	for line := range strings.SplitSeq(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			allowed[strings.ToLower(line)] = true
		}
	}

	return
}

// To returns the bare addresses of every To and Cc recipient, ready for RCPT
// TO. The headers are parsed as RFC 5322 address lists, so display names are
// dropped ("Mario Rossi <m@x.it>" yields "m@x.it") and a comma inside a quoted
// display name ("Rossi, Mario" <m@x.it>) does not split the address. It fails
// if a header cannot be parsed or if there is no recipient at all.
func (m *message) To() (tos []string, err error) {
	// Header is case-insensitive, so "To"/"Cc" already cover any casing.
	for _, h := range []string{"To", "Cc"} {
		value := strings.TrimSpace(m.Header(h))
		if value == "" {
			continue
		}

		var list []*mail.Address
		if list, err = mail.ParseAddressList(value); err != nil {
			return nil, fmt.Errorf("%s header: %w", h, err)
		}

		for _, a := range list {
			tos = append(tos, a.Address)
		}
	}

	if len(tos) == 0 {
		err = errors.New("no recipient")
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

		if !strings.HasSuffix(nx, EXT) {
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
	dst := fmt.Sprintf("%s/%d_%s", targetDir, time.Now().UnixNano(), name)

	// os.Rename cannot move across filesystems (EXDEV, e.g. on BSD when the
	// queues live on different devices); fall back to copy + remove.
	if err := os.Rename(path, dst); err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}

	if err := copyFile(path, dst); err != nil {
		return err
	}

	return os.Remove(path)
}

// copyFile copies the contents of src into a freshly created dst.
func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer utility.Deferrable(in.Close, nil, nil)

	out, err := os.Create(dst)
	if err != nil {
		return
	}

	if _, err = io.Copy(out, in); err != nil {
		utility.Deferrable(out.Close, nil, nil)
		return
	}

	return out.Close()
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
