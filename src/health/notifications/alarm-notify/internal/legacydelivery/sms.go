// SPDX-License-Identifier: GPL-3.0-or-later

package legacydelivery

import (
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/kavenegar"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/messagebird"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/smseagle"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/twilio"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

func buildTwilio(b *builder, recipients []string) ([]notifier.Sender, error) {
	return each(recipients, func(r string) (notifier.Sender, error) {
		return twilio.New(twilio.Config{Secrets: secret.LiteralInput, AccountSID: b.values["TWILIO_ACCOUNT_SID"], AuthToken: b.values["TWILIO_ACCOUNT_TOKEN"], From: b.values["TWILIO_NUMBER"], To: r}, b.client)
	})
}
func buildMessageBird(b *builder, recipients []string) ([]notifier.Sender, error) {
	return each(recipients, func(r string) (notifier.Sender, error) {
		return messagebird.New(messagebird.Config{Secrets: secret.LiteralInput, AccessKey: b.values["MESSAGEBIRD_ACCESS_KEY"], Originator: b.values["MESSAGEBIRD_NUMBER"], Recipient: r}, b.client)
	})
}
func buildKavenegar(b *builder, recipients []string) ([]notifier.Sender, error) {
	return each(recipients, func(r string) (notifier.Sender, error) {
		return kavenegar.New(kavenegar.Config{Secrets: secret.LiteralInput, APIKey: b.values["KAVENEGAR_API_KEY"], Sender: b.values["KAVENEGAR_SENDER"], Recipient: r}, b.client)
	})
}
func buildSMSEagle(b *builder, recipients []string) ([]notifier.Sender, error) {
	cfg := smseagle.Config{Secrets: secret.LiteralInput, APIURL: b.values["SMSEAGLE_API_URL"], AccessToken: b.values["SMSEAGLE_API_ACCESSTOKEN"], MessageType: b.values["SMSEAGLE_MSG_TYPE"], Recipients: recipients}
	var err error
	switch cfg.MessageType {
	case "ring", "tts", "tts_advanced":
		cfg.CallDuration, err = integer(b.values["SMSEAGLE_CALL_DURATION"], "SMSEAGLE_CALL_DURATION")
		if err != nil {
			return nil, err
		}
	}
	if cfg.MessageType == "tts_advanced" {
		cfg.VoiceID, err = integer(b.values["SMSEAGLE_VOICE_ID"], "SMSEAGLE_VOICE_ID")
		if err != nil {
			return nil, err
		}
	}
	return one(smseagle.New(cfg, b.client))
}
