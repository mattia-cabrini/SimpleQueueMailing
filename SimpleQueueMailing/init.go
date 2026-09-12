// Copyright (c) 2026 Mattia Cabrini
// SPDX-License-Identifier: MIT

package SimpleQueueMailing

import (
	"crypto/tls"
	_ "embed"
	"errors"
	"fmt"
	"github.com/mattia-cabrini/go-utility"
	"net/smtp"
	"net/textproto"
	"os"
	"time"
)

// errServerFault marks a sending failure that is not the message's fault: the
// server is unreachable, dropped the connection, refused the login or sender,
// or answered with a temporary (4xx) reply. Such a message stays in the input
// queue and is retried after a growing pause (see serverFaultBackoff).
var errServerFault = errors.New("SMTP server fault")

// serverFaultBackoff grows the pause before retrying while the SMTP server
// keeps failing: ServerFaultPauseMin after the first fault, then twice the
// previous pause at every further one, capped at ServerFaultPauseMax.
type serverFaultBackoff struct {
	faults int           // consecutive server faults
	pause  time.Duration // pause taken after the latest fault
}

// next records a server fault and returns the pause to take before retrying.
func (b *serverFaultBackoff) next(conf *Config) time.Duration {
	if b.faults == 0 {
		b.pause = conf.serverFaultPauseMin()
	} else {
		b.pause = min(2*b.pause, conf.serverFaultPauseMax())
	}

	b.faults++
	return b.pause
}

// reset forgets past faults once a message has been sent successfully.
func (b *serverFaultBackoff) reset() {
	if b.faults > 0 {
		logf(utility.WARNING, "SMTP server working again after %d fault(s)", b.faults)
	}

	*b = serverFaultBackoff{}
}

func ExecuteMailing(conf *Config, backoff *serverFaultBackoff) {
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

	err = sendMessage(conf, &m)
	if errors.Is(err, errServerFault) {
		pause := backoff.next(conf)
		logf(utility.WARNING,
			"SMTP server fault #%d sending mail %s, pausing %v before retrying - %s",
			backoff.faults, m.Re(), pause, err.Error(),
		)
		time.Sleep(pause)
		return
	}

	if err != nil {
		logf(utility.ERROR, "Could not send mail %s - %s", m.Re(), err.Error())
		reject(conf, &m, name, path, "could not send: "+err.Error())
		return
	}

	backoff.reset()

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

	// Deferred first, so it runs last: the pause starts once the SMTP session
	// is closed.
	defer time.Sleep(conf.sendPause())

	host := fmt.Sprintf("%s:%d", conf.SmtpServer, conf.SmtpPort)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: conf.SmtpInsecureSkipVerify,
		ServerName:         conf.SmtpServer,
	}

	conn, err := tls.Dial("tcp", host, tlsConfig)
	if err != nil {
		return fmt.Errorf("%w: tls dial failed: %w", errServerFault, err)
	}

	client, err := smtp.NewClient(conn, conf.SmtpServer)
	if err != nil {
		closeLogged(conn.Close, "TLS connection to "+host)
		return fmt.Errorf("%w: could not create new client: %w", errServerFault, err)
	}
	defer quitSMTP(client, host)

	// Up to MAIL FROM nothing depends on the message, so any failure is the
	// server's (or the configuration's).
	auth := smtp.PlainAuth("", conf.Sender, conf.Password, conf.SmtpServer)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("%w: plain auth failed: %w", errServerFault, err)
	}

	if err = client.Mail(conf.Sender); err != nil {
		return fmt.Errorf("%w: set sender failed: %w", errServerFault, err)
	}

	for _, addr := range rcpts {
		if err = client.Rcpt(addr); err != nil {
			return serverFaultUnlessPermanent(fmt.Errorf("set recipient %s failed: %w", addr, err))
		}
	}

	w, err := client.Data()
	if err != nil {
		return serverFaultUnlessPermanent(fmt.Errorf("could not init writer: %w", err))
	}

	if err = m.PrintTo(conf, w); err != nil {
		return serverFaultUnlessPermanent(fmt.Errorf("could not write message: %w", err))
	}

	if err = w.Close(); err != nil {
		return serverFaultUnlessPermanent(fmt.Errorf("could not close writer: %w", err))
	}

	return nil
}

// serverFaultUnlessPermanent marks err, a failure while transmitting the
// message, as a server fault unless it is a permanent (5xx) SMTP reply: only
// that refuses this very message, while a dropped connection or a temporary
// (4xx) reply is worth retrying.
func serverFaultUnlessPermanent(err error) error {
	var reply *textproto.Error
	if errors.As(err, &reply) && reply.Code >= 500 {
		return err
	}

	return fmt.Errorf("%w: %w", errServerFault, err)
}

// quitSMTP ends the SMTP session. When QUIT fails (e.g. the server already hung
// up, which surfaces as EOF) net/smtp leaves the connection open, so it is
// closed explicitly. The failure is only logged: by then the message has
// already been accepted or refused.
func quitSMTP(client *smtp.Client, host string) {
	if err := client.Quit(); err != nil {
		logf(utility.WARNING, "SMTP QUIT to %s failed, closing the connection - %s", host, err.Error())
		closeLogged(client.Close, "SMTP connection to "+host)
	}
}

func App() {
	printHelp()
	printSampleConfig()

	conf := readConfig()

	fatalIf(conf.Check(), "Invalid configuration")

	if conf.SmtpInsecureSkipVerify {
		logf(utility.WARNING, "SMTP TLS certificate verification is disabled (SmtpInsecureSkipVerify)")
	}

	var backoff serverFaultBackoff

	for {
		ExecuteMailing(&conf, &backoff)
		time.Sleep(100 * time.Millisecond)
	}
}
