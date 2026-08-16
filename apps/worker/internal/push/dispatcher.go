package push

import (
	"context"
	"errors"
)

type Repository interface {
	Pending(context.Context, string) ([]Notification, error)
	MarkSent(context.Context, Notification, string) error
	MarkRetry(context.Context, Notification, error) error
	DisableInvalidToken(context.Context, Notification, error) error
}

type Dispatcher struct {
	Repository Repository
	Provider   Provider
}

func (d Dispatcher) Dispatch(ctx context.Context, notificationID string) error {
	if d.Repository == nil || d.Provider == nil {
		return errors.New("push dispatcher is misconfigured")
	}
	deliveries, err := d.Repository.Pending(ctx, notificationID)
	if err != nil {
		return err
	}
	var retryable error
	for _, delivery := range deliveries {
		apnsID, sendErr := d.Provider.Send(ctx, delivery)
		switch {
		case sendErr == nil:
			err = d.Repository.MarkSent(ctx, delivery, apnsID)
		case errors.Is(sendErr, ErrInvalidDeviceToken):
			err = d.Repository.DisableInvalidToken(ctx, delivery, sendErr)
		default:
			err = d.Repository.MarkRetry(ctx, delivery, sendErr)
			retryable = sendErr
		}
		if err != nil {
			return err
		}
	}
	return retryable
}
