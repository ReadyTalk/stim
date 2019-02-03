package stim

import (
	"github.com/readytalk/stim/pkg/slack"
)

func (stim *Stim) Slack() *slack.Slack {
	stim.log.Debug("Stim-Slack: Creating")

	vault := stim.Vault()
	token, err := vault.GetSecretKey("secret/slack/stimbot", "apikey")
	if err != nil {
		stim.log.Fatal(err)
	}

	s, err := slack.New(&slack.Config{Token: token, Logger: stim.log})
	if err != nil {
		stim.log.Fatal("Stim-Slack: Error Initializaing: ", err)
	}

	return s
}