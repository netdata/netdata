// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

type signl4Event struct {
	Title        string `json:"Title"`
	Message      string `json:"Message"`
	Severity     string `json:"Severity"`
	ExternalID   string `json:"X-S4-ExternalID"`
	Status       string `json:"X-S4-Status"`
	SourceSystem string `json:"X-S4-SourceSystem"`
}

func renderSIGNL4(event Event) signl4Event {
	status := "new"
	if event.Status == "CLEAR" {
		status = "resolved"
	}
	return signl4Event{
		Title:   event.Node + " " + event.Status + ": " + event.Summary,
		Message: notificationPlainText(event, true), Severity: event.Status,
		ExternalID: event.IncidentID, Status: status, SourceSystem: "Netdata",
	}
}
