// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func (dst Destination) validateEmail() error {
	allowed := Destination{Type: dst.Type, Executable: dst.Executable, Env: dst.Env,
		Recipients: dst.Recipients, From: dst.From, PlainTextOnly: dst.PlainTextOnly, Threading: dst.Threading}
	if !reflect.DeepEqual(dst, allowed) {
		return errors.New("email destination contains fields for another provider")
	}
	command := Destination{Type: "command", Executable: dst.Executable, Env: dst.Env}
	if err := command.validateCommand(); err != nil {
		return fmt.Errorf("email: %w", err)
	}
	if len(dst.Recipients) == 0 {
		return errors.New("email requires at least one recipient")
	}
	for _, recipient := range dst.Recipients {
		if _, _, err := emailAddress(recipient); err != nil {
			return fmt.Errorf("email recipients: %w", err)
		}
	}
	if dst.From != "" {
		if _, _, err := emailAddress(dst.From); err != nil {
			return fmt.Errorf("email from: %w", err)
		}
	}
	return nil
}

// Return a safe header and an envelope address, preserving quoted mailbox local parts.
func emailAddress(raw string) (header, envelope string, err error) {
	invalid := errors.New("expected a literal mailbox or safe local alias without control characters")
	if !utf8.ValidString(raw) || strings.Contains(raw, "${") || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return "", "", invalid
	}
	alias := raw != "" && len(raw) <= 990
	for i, c := range raw {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' ||
			(i > 0 && (c == '.' || c == '-' || c == '+'))) {
			alias = false
			break
		}
	}
	if alias {
		return raw, raw, nil
	}
	address, err := mail.ParseAddress(raw)
	if err != nil || !utf8.ValidString(address.Name+address.Address) || strings.ContainsAny(address.Address[:1], "-|/") ||
		strings.IndexFunc(address.Name+address.Address, unicode.IsControl) >= 0 {
		return "", "", invalid
	}
	// Address.String handles quoting; display names are encoded separately so long ASCII names also fold.
	mailbox := (&mail.Address{Address: address.Address}).String()
	if len(mailbox) > 990 {
		return "", "", invalid
	}
	header = mailbox
	if address.Name != "" {
		header = emailEncodedWords(address.Name) + "\r\n " + mailbox
	}
	return header, strings.TrimSuffix(strings.TrimPrefix(mailbox, "<"), ">"), nil
}

// Encode even ASCII text: an unbroken alert name must not exceed mail's header line limit.
func emailEncodedWords(value string) string {
	var words []string
	for len(value) > 0 {
		n := min(45, len(value))
		for n < len(value) && !utf8.RuneStart(value[n]) {
			n--
		}
		words = append(words, "=?UTF-8?B?"+base64.StdEncoding.EncodeToString([]byte(value[:n]))+"?=")
		value = value[n:]
	}
	return strings.Join(words, "\r\n ")
}

func renderEmail(dst Destination, event Event) ([]string, []byte, error) {
	var message bytes.Buffer
	args := []string{"-t", "-i"}
	var recipients []string
	for _, raw := range dst.Recipients {
		header, _, err := emailAddress(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("email recipients: %w", err)
		}
		recipients = append(recipients, header)
	}
	fmt.Fprintf(&message, "To:\r\n %s\r\n", strings.Join(recipients, ",\r\n "))
	if dst.From != "" {
		header, envelope, err := emailAddress(dst.From)
		if err != nil {
			return nil, nil, fmt.Errorf("email from: %w", err)
		}
		fmt.Fprintf(&message, "From:\r\n %s\r\n", header)
		args = append(args, "-f", envelope)
	}
	status := "needs attention"
	switch event.Status {
	case "CRITICAL":
		status = "is critical"
	case "CLEAR":
		status = "recovered"
	}
	subject := event.Node + " " + status + ": " + strings.ReplaceAll(event.Alert, "_", " ")
	if event.Chart != "" {
		subject += " (" + event.Chart + ")"
	}
	fmt.Fprintf(&message, "Subject:\r\n %s\r\n", emailEncodedWords(subject))
	fmt.Fprintf(&message, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&message, "Message-ID: <%s@netdata.invalid>\r\n", rand.Text())
	fmt.Fprintf(&message, "X-Netdata-Severity: %s\r\n", strings.ToLower(event.Status))
	for _, field := range []struct{ name, value string }{
		{"X-Netdata-Alert-Name", event.Alert}, {"X-Netdata-Chart", event.Chart}, {"X-Netdata-Host", event.Node},
	} {
		fmt.Fprintf(&message, "%s:\r\n %s\r\n", field.name, emailEncodedWords(field.value))
	}
	if dst.Threading == nil || *dst.Threading {
		// Structured encoding avoids delimiter collisions; Bash groups by chart, alert and node, not incident.
		identity, _ := json.Marshal([3]string{event.Node, event.Chart, event.Alert})
		thread := fmt.Sprintf("<netdata.%x@netdata.invalid>", sha256.Sum256(identity))
		fmt.Fprintf(&message, "In-Reply-To: %s\r\nReferences: %s\r\n", thread, thread)
	}
	message.WriteString("MIME-Version: 1.0\r\n")
	plain := notificationPlainText(event, true) + "\nIncident ID: " + event.IncidentID
	for _, duration := range []struct {
		name  string
		value *uint32
	}{{"Duration", event.Duration}, {"Non-clear duration", event.NonClearDuration}} {
		if duration.value != nil {
			plain += "\n" + duration.name + ": " + strconv.FormatUint(uint64(*duration.value), 10) + " seconds"
		}
	}
	if dst.PlainTextOnly != nil && *dst.PlainTextOnly {
		message.WriteString("Content-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
		writeEmailText(&message, plain)
	} else {
		parts := multipart.NewWriter(&message)
		fmt.Fprintf(&message, "Content-Type: %s\r\n\r\n", mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": parts.Boundary()}))
		htmlBody := "<!DOCTYPE html><html><body><pre>" + html.EscapeString(plain) + "</pre>"
		if event.URL != "" {
			htmlBody += `<p><a href="` + html.EscapeString(event.URL) + `">View in Netdata</a></p>`
		}
		htmlBody += "</body></html>"
		for _, part := range []struct{ kind, body string }{{"text/plain", plain}, {"text/html", htmlBody}} {
			// bytes.Buffer writes cannot fail; the headers and boundary are generated locally.
			writer, _ := parts.CreatePart(textproto.MIMEHeader{
				"Content-Type": {part.kind + "; charset=UTF-8"}, "Content-Transfer-Encoding": {"quoted-printable"},
			})
			writeEmailText(writer, part.body)
		}
		_ = parts.Close()
	}
	return args, message.Bytes(), nil
}

func writeEmailText(w io.Writer, text string) {
	// MIME text uses CRLF; quoted-printable wraps long lines and transports UTF-8 on 7-bit MTAs.
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = strings.ReplaceAll(text, "\n", "\r\n") + "\r\n"
	encoded := quotedprintable.NewWriter(w)
	_, _ = io.WriteString(encoded, text)
	_ = encoded.Close()
}

func sendEmail(ctx context.Context, processes *commandProcesses, dst Destination, event Event) error {
	env, err := commandEnvironment(ctx, dst.Env)
	if err != nil {
		return err
	}
	args, message, err := renderEmail(dst, event)
	if err != nil {
		return err
	}
	return processes.run(ctx, dst.Executable, args, env, bytes.NewReader(message))
}
