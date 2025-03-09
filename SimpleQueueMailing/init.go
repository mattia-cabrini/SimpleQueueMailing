// Copyright (c) 2023 Mattia Cabrini
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
				m.Re(), err.Error(),
			)
			return
		}

		client, err := smtp.NewClient(conn, conf.SmtpServer)
		if err != nil {
			defer utility.Deferrable(conn.Close, nil, nil)
			utility.Logf(utility.ERROR,
				"Could not send mail (could not create new client) %s - %s",
				m.Re(), err.Error(),
			)
			return
		}
		defer utility.Deferrable(client.Quit, nil, nil)

		auth := smtp.PlainAuth("", conf.Sender, conf.Password, conf.SmtpServer)
		if err = client.Auth(auth); err != nil {
			utility.Logf(utility.ERROR,
				"Could not send mail (plain auth failed) %s - %s",
				m.Re(), err.Error(),
			)
			return
		}

		if err = client.Mail(conf.Sender); err != nil {
			utility.Logf(utility.ERROR,
				"Could not send mail (set sender failed) %s - %s",
				m.Re(), err.Error(),
			)
			return
		}

		for _, addr := range m.To() {
			if err = client.Rcpt(addr); err != nil {
				utility.Logf(utility.ERROR,
					"Could not send mail (set receipient %s failed) %s - %s",
					addr, m.Re(), err.Error(),
				)
				return
			}
		}

		w, err := client.Data()
		if err != nil {
			utility.Logf(utility.ERROR,
				"Could not send mail (could not init writer) %s - %s",
				m.Re(), err.Error(),
			)
			return
		}
		err = m.PrintTo(conf, w)
		if err != nil {
			utility.Logf(utility.ERROR,
				"Could not send mail (could not write message) %s - %s",
				m.Re(), err.Error(),
			)
			return
		}
		err = w.Close()
		if err != nil {
			utility.Logf(utility.ERROR,
				"Could not send mail (could not close writer) %s - %s",
				m.Re(), err.Error(),
			)
			return
		}

		// err = smtp.SendMail(host, auth, conf.Sender, m.To(), m.Msg(conf))
		// if err != nil {
		// 	utility.Logf(utility.ERROR, "Could not send mail %s - %s", m.Re, err.Error())
		// 	return
		// }
		utility.Logf(utility.WARNING, "Sent mail %s to %v", m.Re(), m.To())
	}
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
