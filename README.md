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

## TLS

The SMTP server certificate is verified. `SmtpInsecureSkipVerify: true`
disables the check, exposing the connection and the SMTP password to
man-in-the-middle attacks: use it only for testing.
