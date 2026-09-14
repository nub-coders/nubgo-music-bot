package telegrambot

import (
	"context"
	"errors"

	"github.com/nub-coders/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/storage"
)

type Authorizer struct {
	client  *telegram.Client
	store   storage.Access
	botID   int64
	ownerID int64
}

func NewAuthorizer(client *telegram.Client, store storage.Access, botID, ownerID int64) *Authorizer {
	return &Authorizer{client: client, store: store, botID: botID, ownerID: ownerID}
}

func (a *Authorizer) CanControl(ctx context.Context, chatID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	blocked, err := a.store.IsBlocked(ctx, a.botID, userID)
	if err != nil {
		return false, err
	}
	if blocked {
		return false, nil
	}
	if userID == a.ownerID && a.ownerID > 0 {
		return true, nil
	}
	sudo, err := a.store.IsSudo(ctx, a.botID, userID)
	if err != nil {
		return false, err
	}
	if sudo {
		return true, nil
	}
	botAdmin, err := a.store.IsBotAdmin(ctx, a.botID, userID)
	if err != nil {
		return false, err
	}
	if botAdmin {
		return true, nil
	}
	admin, err := a.isChatAdmin(chatID, userID)
	if err == nil && admin {
		return true, nil
	}
	authorized, storeErr := a.store.IsAuthorized(ctx, a.botID, chatID, userID)
	if storeErr != nil {
		return false, storeErr
	}
	return authorized, nil
}

func (a *Authorizer) CanManage(ctx context.Context, chatID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	if userID == a.ownerID && a.ownerID > 0 {
		return true, nil
	}
	sudo, err := a.store.IsSudo(ctx, a.botID, userID)
	if err != nil {
		return false, err
	}
	if sudo {
		return true, nil
	}
	botAdmin, err := a.store.IsBotAdmin(ctx, a.botID, userID)
	if err != nil {
		return false, err
	}
	if botAdmin {
		return true, nil
	}
	return a.isChatAdmin(chatID, userID)
}

func (a *Authorizer) isChatAdmin(chatID, userID int64) (bool, error) {
	participant, err := a.client.GetChatMember(chatID, userID)
	if err != nil {
		return false, err
	}
	if participant == nil {
		return false, errors.New("Telegram returned an empty chat participant")
	}
	return participant.Status == telegram.Admin || participant.Status == telegram.Creator, nil
}
