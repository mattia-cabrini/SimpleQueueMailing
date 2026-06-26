// Copyright (c) 2026 Mattia Cabrini
// SPDX-License-Identifier: MIT

package SimpleQueueMailing

import (
	"crypto/tls"
	_ "embed"
	"fmt"
	"github.com/mattia-cabrini/go-utility"
	"net/smtp"
	"time"
)

func ExecuteMailing(conf *Config) {
	m, found, name, path, err := CreateMessageFrom(conf)

	if !found {
		if err != nil {
			utility.Logf(utility.ERROR, "Could not scan input queue - %s", err.Error())
		}
		return
	}

	if err != nil {
		utility.Logf(utility.ERROR, "Could not read message %s - %s", name, err.Error())
		reject(conf, name, path)
		return
	}

	if err = sendMessage(conf, &m); err != nil {
		utility.Logf(utility.ERROR, "Could not send mail %s - %s", m.Re(), err.Error())
		reject(conf, name, path)
		return
	}

	if err = MoveToQueue(conf.QueueOut, name, path); err != nil {
		utility.Logf(utility.ERROR,
			"Sent mail %s but could not move it to the output queue - %s",
			m.Re(), err.Error(),
		)
		return
	}

	utility.Logf(utility.WARNING, "Sent mail %s to %v", m.Re(), m.To())
}

// reject moves a message that could not be read or sent into the rejected
// queue, so a poison message stops blocking the rest of the input queue.
func reject(conf *Config, name string, path string) {
	if err := MoveToQueue(conf.QueueRejected, name, path); err != nil {
		utility.Logf(utility.ERROR,
			"Could not move rejected message %s to the rejected queue - %s",
			name, err.Error(),
		)
		return
	}

	utility.Logf(utility.WARNING, "Rejected message %s, moved to the rejected queue", name)
}

func sendMessage(conf *Config, m *message) (err error) {
	host := fmt.Sprintf("%s:%d", conf.SmtpServer, conf.SmtpPort)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // Da usare solo in ambiente di test
		ServerName:         conf.SmtpServer,
	}

	conn, err := tls.Dial("tcp", host, tlsConfig)
	if err != nil {
		return fmt.Errorf("tls dial failed: %w", err)
	}

	client, err := smtp.NewClient(conn, conf.SmtpServer)
	if err != nil {
		defer utility.Deferrable(conn.Close, nil, nil)
		return fmt.Errorf("could not create new client: %w", err)
	}
	defer utility.Deferrable(client.Quit, nil, nil)

	auth := smtp.PlainAuth("", conf.Sender, conf.Password, conf.SmtpServer)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("plain auth failed: %w", err)
	}

	if err = client.Mail(conf.Sender); err != nil {
		return fmt.Errorf("set sender failed: %w", err)
	}

	for _, addr := range m.To() {
		if err = client.Rcpt(addr); err != nil {
			return fmt.Errorf("set recipient %s failed: %w", addr, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("could not init writer: %w", err)
	}

	if err = m.PrintTo(conf, w); err != nil {
		return fmt.Errorf("could not write message: %w", err)
	}

	if err = w.Close(); err != nil {
		return fmt.Errorf("could not close writer: %w", err)
	}

	return nil
}

func App() {
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
