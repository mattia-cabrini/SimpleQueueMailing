# Simple Queue Mailing

Takes all .noeml (NOt ad EML) files in a directory and sends them via email.

## Writing to the input queue

Every file whose name ends in `noeml` is picked up as soon as it appears in
`QueueIn`, even if it is still being written: it would be sent truncated. A
producer must therefore write each message under a temporary name that does not
end in `noeml` (e.g. `msg.noeml.tmp`), in `QueueIn` itself or at least on the
same filesystem, and then rename it to its final name. On POSIX systems a
rename within one filesystem is atomic, so the message appears either complete
or not at all.

## Recipients

`To` and `Cc` are parsed as RFC 5322 address lists: bare addresses
(`m@x.it`), addresses with a display name (`Mario Rossi <m@x.it>`) and quoted
display names containing commas (`"Rossi, Mario" <m@x.it>`) are all accepted.
A message with an unparsable recipient list, or with no recipient at all, is
moved to `QueueRejected`.

## Sending rate

After every delivery attempt the program pauses for `SendPause` milliseconds
(default `6000`; `0` disables the pause).

When the SMTP server is unreachable, refuses the login or answers with a
temporary error, the message stays in `QueueIn` and is retried with an
exponential backoff: the first pause lasts `ServerFaultPauseMin` milliseconds
(default `1000`), and every further consecutive fault doubles it, up to
`ServerFaultPauseMax` (default `3600000`, i.e. 1 hour). As soon as a message is
sent successfully the pause goes back to the minimum. Every pause is logged as
a warning.

## TLS

The SMTP server certificate is verified. `SmtpInsecureSkipVerify: true`
disables the check, exposing the connection and the SMTP password to
man-in-the-middle attacks: use it only for testing.
