// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
)

type deliveryResult struct {
	destination string
	skipReason  string
	err         error
}

func dispatch(
	ctx context.Context,
	processes *commandexec.Runner,
	cfg Config,
	destinations []string,
	event notifyevent.Event,
	timeout time.Duration,
	report func(deliveryResult),
) error {
	succeeded, attempted := 0, 0
	for _, name := range destinations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if reason := cfg.Routing.Policies[name].skipReason(event.Status); reason != "" {
			report(deliveryResult{destination: name, skipReason: reason})
			continue
		}
		attempted++
		dst := cfg.Destinations[name]
		var err error
		switch dst.Type {
		case "irc":
			err = sendIRC(ctx, processes, dst, event)
		case "email":
			err = sendEmail(ctx, processes, dst, event)
		case "kafka":
			err = postJSON(ctx, dst, renderKafka(dst, event), timeout)
		case "syslog":
			err = sendSyslog(ctx, processes, dst, event)
		case "awssns":
			err = sendAWSSNS(ctx, processes, dst, event)
		case "command", "smstools3":
			err = sendCommand(ctx, processes, dst, event)
		case "webhook":
			err = sendWebhook(ctx, dst, event, timeout)
		case "slack":
			err = sendSlack(ctx, dst, event, timeout)
		case "discord":
			err = sendDiscord(ctx, dst, event, timeout)
		case "telegram":
			err = sendTelegram(ctx, dst, event, timeout)
		case "pushover":
			err = sendPushover(ctx, dst, event, timeout)
		case "pushbullet":
			err = sendPushbullet(ctx, dst, event, timeout)
		case "twilio":
			err = sendTwilio(ctx, dst, event, timeout)
		case "messagebird":
			err = sendMessageBird(ctx, dst, event, timeout)
		case "gotify":
			err = sendGotify(ctx, dst, event, timeout)
		case "ntfy":
			err = sendNtfy(ctx, dst, event, timeout)
		case "rocketchat":
			err = postJSON(ctx, dst, renderRocketChat(dst, event), timeout)
		case "flock":
			err = postJSON(ctx, dst, renderFlock(event), timeout)
		case "fleep":
			err = postJSON(ctx, dst, renderFleep(dst, event), timeout)
		case "ilert":
			err = sendIlert(ctx, dst, event, timeout)
		case "signl4":
			err = postJSON(ctx, dst, renderSIGNL4(event), timeout)
		case "alerta":
			err = sendAlerta(ctx, dst, event, timeout)
		case "pagerduty":
			err = sendPagerDuty(ctx, dst, event, timeout)
		case "opsgenie":
			err = sendOpsgenie(ctx, dst, event, timeout)
		case "msteams":
			err = sendMSTeams(ctx, dst, event, timeout)
		case "matrix":
			err = sendMatrix(ctx, dst, event, timeout)
		case "smseagle":
			err = sendSMSEagle(ctx, dst, event, timeout)
		case "prowl":
			err = sendProwl(ctx, dst, event, timeout)
		case "kavenegar":
			err = sendKavenegar(ctx, dst, event, timeout)
		case "dynatrace":
			err = sendDynatrace(ctx, dst, event, timeout)
		default:
			err = errors.New("destination provider is not implemented")
		}
		report(deliveryResult{destination: name, err: err})
		if err == nil {
			succeeded++
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if attempted > 0 && succeeded == 0 {
		return errors.New("all attempted destinations failed")
	}
	return nil
}
