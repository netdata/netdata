// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
)

func (dst Destination) validateIRC() error {
	allowed := Destination{Type: dst.Type, Executable: dst.Executable, Env: dst.Env, Host: dst.Host,
		Port: dst.Port, Nickname: dst.Nickname, Realname: dst.Realname, Channel: dst.Channel}
	if !reflect.DeepEqual(dst, allowed) {
		return errors.New("irc destination contains fields for another provider")
	}
	if err := (Destination{Type: "command", Executable: dst.Executable, Env: dst.Env}).validateCommand(); err != nil {
		return fmt.Errorf("irc: %w", err)
	}
	if dst.Host == "" || !validCommandHost(dst.Host) {
		return errors.New("irc host must be a literal hostname or unbracketed IP address without whitespace or options")
	}
	if dst.Port != nil && (*dst.Port < 1 || *dst.Port > 65535) {
		return errors.New("irc port must be an integer from 1 to 65535")
	}
	if !validIRCNickname(dst.Nickname) || len("NICK "+dst.Nickname)+2 > 512 {
		return errors.New("irc nickname must be a protocol-safe nickname fitting one IRC line")
	}
	if strings.TrimSpace(dst.Realname) == "" || !utf8.ValidString(dst.Realname) || strings.Contains(dst.Realname, "${") ||
		strings.IndexFunc(dst.Realname, unicode.IsControl) >= 0 || len("USER "+dst.Nickname+" 0 * :"+dst.Realname)+2 > 512 {
		return errors.New("irc realname must be literal nonempty text without controls fitting the USER line")
	}
	if !validIRCChannel(dst.Channel) {
		return errors.New("irc channel must start with #, &, + or ! and contain at most 50 bytes without spaces, controls or commas")
	}
	return nil
}

func validIRCNickname(nick string) bool {
	if nick == "" {
		return false
	}
	for i, c := range nick {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || strings.ContainsRune("[]\\`_^{}|", c) ||
			(i > 0 && ((c >= '0' && c <= '9') || c == '-')) {
			continue
		}
		return false
	}
	return true
}

func validIRCChannel(channel string) bool {
	return len(channel) > 1 && len(channel) <= 50 && utf8.ValidString(channel) && strings.ContainsAny(channel[:1], "#&+!") &&
		!strings.Contains(channel, ",") && !strings.Contains(channel, "${") && strings.IndexFunc(channel, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) < 0
}

func renderIRC(event notifyevent.Event) string {
	// Preserve literal backslashes; flatten newlines without letting text become IRC/CTCP commands.
	text := notifymsg.PlainText(event, true)
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	return notifymsg.EscapeControls(strings.ReplaceAll(text, "\n", ", "))
}

type ircMessage struct {
	prefix, command string
	params          []string
}

func readIRC(reader *bufio.Reader) (ircMessage, error) {
	line, err := reader.ReadSlice('\n')
	if err != nil {
		if errors.Is(err, io.EOF) && len(line) == 0 {
			return ircMessage{}, io.EOF
		}
		return ircMessage{}, errors.New("irc could not read a complete bounded protocol line")
	}
	text := strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r")
	if strings.ContainsAny(text, "\r\n\x00") {
		return ircMessage{}, errors.New("irc received invalid protocol framing")
	}
	if strings.HasPrefix(text, "@") {
		end := strings.IndexByte(text, ' ')
		if end < 0 || end > 8191 {
			return ircMessage{}, errors.New("irc received invalid message tags")
		}
		text = text[end+1:]
	}
	if len(text)+2 > 512 {
		return ircMessage{}, errors.New("irc received an overlong protocol line")
	}
	var result ircMessage
	text = strings.TrimLeft(text, " ")
	if text == "" {
		return result, nil
	}
	if text[0] == ':' {
		prefix, rest, ok := strings.Cut(text[1:], " ")
		if !ok || prefix == "" {
			return ircMessage{}, errors.New("irc received an invalid protocol prefix")
		}
		result.prefix, text = prefix, strings.TrimLeft(rest, " ")
	}
	command, rest, _ := strings.Cut(text, " ")
	result.command = strings.ToUpper(command)
	letters, digits := command != "", len(command) == 3
	for _, c := range command {
		letters = letters && ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'))
		digits = digits && c >= '0' && c <= '9'
	}
	if !letters && !digits {
		return ircMessage{}, errors.New("irc received an invalid protocol command")
	}
	for rest = strings.TrimLeft(rest, " "); rest != ""; rest = strings.TrimLeft(rest, " ") {
		if len(result.params) == 15 {
			return ircMessage{}, errors.New("irc received too many protocol parameters")
		}
		if rest[0] == ':' {
			result.params = append(result.params, rest[1:])
			break
		}
		var param string
		param, rest, _ = strings.Cut(rest, " ")
		result.params = append(result.params, param)
	}
	return result, nil
}

func ircFold(text, mapping string) string {
	return strings.Map(func(c rune) rune {
		if c >= 'A' && c <= 'Z' {
			return c + 'a' - 'A'
		}
		if mapping != "ascii" {
			switch c {
			case '[':
				return '{'
			case ']':
				return '}'
			case '\\':
				return '|'
			}
		}
		if mapping == "rfc1459" && c == '^' {
			return '~'
		}
		return c
	}, text)
}

func ircSession(dst Destination, text string, reader io.Reader, writer io.WriteCloser) error {
	input := bufio.NewReaderSize(reader, 8192+512)
	write := func(line string) error {
		if len(line)+2 > 512 || strings.ContainsAny(line, "\r\n\x00") {
			return errors.New("irc command exceeds protocol limits")
		}
		if _, err := io.WriteString(writer, line+"\r\n"); err != nil {
			return errors.New("irc could not write protocol command")
		}
		return nil
	}
	if err := write("NICK " + dst.Nickname); err != nil {
		return err
	}
	if err := write("USER " + dst.Nickname + " 0 * :" + dst.Realname); err != nil {
		return err
	}
	state, nick, mapping, token := "register", dst.Nickname, "rfc1459", ""
	relayPrefix := ""
	sendChunk := func() error {
		prefix := "PRIVMSG " + dst.Channel + " :"
		// Reserve the sender prefix the server adds when forwarding the message.
		n := min(len(text), 510-len(prefix)-len(relayPrefix)-2)
		if n <= 0 {
			return errors.New("irc sender prefix leaves no room for message text")
		}
		for n < len(text) && !utf8.RuneStart(text[n]) {
			n--
		}
		if n == 0 {
			return errors.New("irc sender prefix leaves no room for a UTF-8 character")
		}
		if err := write(prefix + text[:n]); err != nil {
			return err
		}
		text = text[n:]
		token = rand.Text()
		return write("PING :" + token)
	}
	for {
		message, err := readIRC(input)
		if errors.Is(err, io.EOF) {
			if state == "quit" {
				return nil
			}
			return errors.New("irc connection closed before exchange completed")
		}
		if err != nil {
			return err
		}
		if code, err := strconv.Atoi(message.command); err == nil && code >= 400 && code <= 599 && code != 422 {
			// 422 (no MOTD) is normal on otherwise usable servers.
			return fmt.Errorf("irc server rejected request with numeric %03d", code)
		}
		same := func(a, b string) bool { return ircFold(a, mapping) == ircFold(b, mapping) }
		sourceNick, _, _ := strings.Cut(message.prefix, "!")
		switch message.command {
		case "PING":
			if state == "quit" {
				continue
			}
			if len(message.params) != 1 || message.params[0] == "" {
				return errors.New("irc received an invalid PING")
			}
			if err := write("PONG :" + message.params[0]); err != nil {
				return err
			}
		case "005":
			for _, param := range message.params {
				if value, ok := strings.CutPrefix(param, "CASEMAPPING="); ok {
					switch value {
					case "ascii", "strict-rfc1459", "rfc1459":
						mapping = value
					default:
						return errors.New("irc server uses an unsupported case mapping")
					}
				}
			}
		case "NICK":
			if same(sourceNick, nick) && len(message.params) == 1 && validIRCNickname(message.params[0]) {
				nick = message.params[0]
				if _, suffix, ok := strings.Cut(relayPrefix, "!"); ok {
					relayPrefix = nick + "!" + suffix
				}
			}
		case "001":
			if state != "register" {
				continue
			}
			if len(message.params) < 1 || !validIRCNickname(message.params[0]) {
				return errors.New("irc received invalid registration confirmation")
			}
			nick = message.params[0]
			if err := write("JOIN " + dst.Channel); err != nil {
				return err
			}
			state = "join"
		case "JOIN":
			if state != "join" || !same(sourceNick, nick) || len(message.params) < 1 || !same(message.params[0], dst.Channel) {
				continue
			}
			relayPrefix = message.prefix
			if err := sendChunk(); err != nil {
				return err
			}
			state = "message"
		case "PONG":
			if state != "message" || len(message.params) == 0 || message.params[len(message.params)-1] != token {
				continue
			}
			if len(text) > 0 {
				if err := sendChunk(); err != nil {
					return err
				}
				continue
			}
			if err := write("QUIT :Netdata notification sent"); err != nil {
				return err
			}
			if err := writer.Close(); err != nil {
				return errors.New("irc could not finish command input")
			}
			state = "quit"
		case "KICK":
			if len(message.params) >= 2 && same(message.params[0], dst.Channel) && same(message.params[1], nick) {
				return errors.New("irc client was removed from the channel")
			}
		case "KILL":
			return errors.New("irc server terminated the client")
		case "ERROR":
			if state != "quit" {
				return errors.New("irc server terminated the connection")
			}
		}
	}
}

func sendIRC(ctx context.Context, processes *commandexec.Runner, dst Destination, event notifyevent.Event) error {
	env, err := commandEnvironment(ctx, dst.Env)
	if err != nil {
		return err
	}
	port := int64(6667)
	if dst.Port != nil {
		port = int64(*dst.Port)
	}
	text := renderIRC(event)
	return processes.RunSession(ctx, dst.Executable, []string{dst.Host, strconv.FormatInt(port, 10)}, env,
		func(reader io.Reader, writer io.WriteCloser) error { return ircSession(dst, text, reader, writer) })
}
