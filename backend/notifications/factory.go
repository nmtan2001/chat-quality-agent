package notifications

// CreateNotifier creates a Notifier based on the OutputConfig
func CreateNotifier(cfg OutputConfig) (Notifier, error) {
	switch cfg.Type {
	case "telegram":
		return NewTelegramNotifier(cfg.BotToken, cfg.ChatID), nil
	case "email":
		return NewEmailNotifier(
			cfg.SMTPHost, cfg.SMTPPort,
			cfg.SMTPUser, cfg.SMTPPass,
			cfg.From, splitComma(cfg.To),
		), nil
	default:
		return nil, ErrUnsupportedOutputType
	}
}

var ErrUnsupportedOutputType = &notifierError{"unsupported output type"}

type notifierError struct {
	msg string
}

func (e *notifierError) Error() string {
	return e.msg
}
