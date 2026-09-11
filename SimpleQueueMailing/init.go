// Copyright (c) 2026 Mattia Cabrini
// SPDX-License-Identifier: MIT

package SimpleQueueMailing

import (
	"crypto/tls"
	_ "embed"
	"fmt"
	"github.com/mattia-cabrini/go-utility"
	"net/smtp"
	"os"
	"time"
)

func ExecuteMailing(conf *Config) {
	m, found, name, path, err := CreateMessageFrom(conf)

	if !found {
		if err != nil {
			logf(utility.ERROR, "Could not scan input queue - %s", err.Error())
		}
		return
	}

	if err != nil {
		logf(utility.ERROR, "Could not read message %s - %s", name, err.Error())
		reject(conf, &m, name, path, "could not read message: "+err.Error())
		return
	}

	rcpts, err := m.To()
	if err != nil {
		logf(utility.ERROR, "Message %s has invalid recipient(s) - %s", name, err.Error())
		reject(conf, &m, name, path, "invalid recipient(s): "+err.Error())
		return
	}

	if !recipientsAuthorized(conf, rcpts) {
		logf(utility.WARNING, "Message %s has unauthorized recipient(s)", name)
		reject(conf, &m, name, path, "unauthorized recipient(s)")
		return
	}

	if err = sendMessage(conf, &m); err != nil {
		logf(utility.ERROR, "Could not send mail %s - %s", m.Re(), err.Error())
		reject(conf, &m, name, path, "could not send: "+err.Error())
		return
	}

	if err = MoveToQueue(conf.QueueOut, name, path); err != nil {
		logf(utility.ERROR,
			"Sent mail %s but could not move it to the output queue - %s",
			m.Re(), err.Error(),
		)
		setAsideSent(conf, name, path)
		return
	}

	logf(utility.WARNING, "Sent mail %s to %v", m.Re(), rcpts)
}

// setAsideSent gets an already sent message out of the input queue when it
// could not be moved to the output queue: left there, it would be picked up and
// sent again on every iteration. It tries the rejected queue first, then renames
// the file in place so that it no longer ends in EXT; if both fail the program
// stops, as that is the only way left to prevent duplicate deliveries.
func setAsideSent(conf *Config, name string, path string) {
	err := MoveToQueue(conf.QueueRejected, name, path)
	if err == nil {
		logf(utility.WARNING, "Sent message %s moved to the rejected queue instead", name)
		return
	}

	logf(utility.ERROR, "Could not move sent message %s to the rejected queue - %s", name, err.Error())

	dst := fmt.Sprintf("%s/%d_%s.sent", conf.QueueIn, time.Now().UnixNano(), name)
	if err = os.Rename(path, dst); err == nil {
		logf(utility.WARNING, "Sent message %s renamed in place to %s", name, dst)
		return
	}

	logf(utility.FATAL,
		"Could not set aside sent message %s, stopping to avoid sending it again - %s",
		name, err.Error(),
	)
}

// reject moves a message that could not be read, is not allowed to be sent, or
// failed to send into the rejected queue, so a poison message stops blocking
// the rest of the input queue. If an Administrator is configured, it is also
// notified about the rejection.
func reject(conf *Config, m *message, name string, path string, reason string) {
	sum, err := fileSHA256(path)
	if err != nil {
		logf(utility.ERROR, "Could not hash rejected message %s - %s", name, err.Error())
	}

	if err := MoveToQueue(conf.QueueRejected, name, path); err != nil {
		logf(utility.ERROR,
			"Could not move rejected message %s to the rejected queue - %s",
			name, err.Error(),
		)
		return
	}

	logf(utility.WARNING, "Rejected message %s (%s), moved to the rejected queue", name, reason)

	notifyAdministrator(conf, m, reason, sum)
}

// notifyAdministrator sends a notification e-mail to conf.Administrator about a
// rejected message. It is a no-op when no administrator is configured.
func notifyAdministrator(conf *Config, m *message, reason string, sha string) {
	if conf.Administrator == "" {
		return
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	now := time.Now().Format(time.RFC1123Z)

	rejectedSubject := ""
	if m != nil {
		rejectedSubject = m.Re()
	}

	notif := message{
		Headers: []string{
			"To: " + conf.Administrator,
			fmt.Sprintf("Subject: SimpleQueueMailing@%s - REJECTED message at %s", hostname, now),
			"Date: " + now,
		},
		Content: []byte(fmt.Sprintf(
			"A message has been rejected.\r\nSubject: %s\r\nReason: %s\r\nSHA256: %s\r\n",
			rejectedSubject, reason, sha,
		)),
	}

	if err := sendMessage(conf, &notif); err != nil {
		logf(utility.ERROR,
			"Could not notify administrator %s about rejected message %q - %s",
			conf.Administrator, rejectedSubject, err.Error(),
		)
	}
}

func sendMessage(conf *Config, m *message) (err error) {
	rcpts, err := m.To()
	if err != nil {
		return fmt.Errorf("invalid recipients: %w", err)
	}

	host := fmt.Sprintf("%s:%d", conf.SmtpServer, conf.SmtpPort)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: conf.SmtpInsecureSkipVerify,
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

	for _, addr := range rcpts {
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

	if conf.SmtpInsecureSkipVerify {
		logf(utility.WARNING, "SMTP TLS certificate verification is disabled (SmtpInsecureSkipVerify)")
	}

	for {
		ExecuteMailing(&conf)
		time.Sleep(100 * time.Millisecond)
	}
}
