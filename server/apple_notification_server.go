// Copyright (c) 2015 Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"fmt"
	"github.com/sideshow/apns2/token"
	"time"

	"github.com/kyokomi/emoji"
	apns "github.com/sideshow/apns2"
	"github.com/sideshow/apns2/certificate"
	"github.com/sideshow/apns2/payload"
	"os"
	"path/filepath"
)

type AppleNotificationServer struct {
	ApplePushSettings ApplePushSettings
	AppleClient       *apns.Client
}

func NewAppleNotificationServer(settings ApplePushSettings) NotificationServer {
	return &AppleNotificationServer{ApplePushSettings: settings}
}

func FindPemFile(fileName string) string {
	if _, err := os.Stat("/tmp/" + fileName); err == nil {
		fileName, _ = filepath.Abs("/tmp/" + fileName)
	} else if _, err := os.Stat("./config/" + fileName); err == nil {
		fileName, _ = filepath.Abs("./config/" + fileName)
	} else if _, err := os.Stat("../config/" + fileName); err == nil {
		fileName, _ = filepath.Abs("../config/" + fileName)
	} else if _, err := os.Stat(fileName); err == nil {
		fileName, _ = filepath.Abs(fileName)
	}

	return fileName
}

func (me *AppleNotificationServer) Initialize() bool {
	LogInfo(fmt.Sprintf("Initializing apple notification server for type=%v", me.ApplePushSettings.Type))

	if len(me.ApplePushSettings.ApplePushCertPrivate) > 0 {

		appleCert, appleCertErr := certificate.FromPemFile(FindPemFile(me.ApplePushSettings.ApplePushCertPrivate), me.ApplePushSettings.ApplePushCertPassword)
		if appleCertErr != nil {
			LogCritical(fmt.Sprintf("Failed to load the apple pem cert err=%v for type=%v", appleCertErr, me.ApplePushSettings.Type))
			return false
		}

		if me.ApplePushSettings.ApplePushUseDevelopment {
			me.AppleClient = apns.NewClient(appleCert).Development()
		} else {
			me.AppleClient = apns.NewClient(appleCert).Production()
		}

		return true
	} else if len(me.ApplePushSettings.ApplePushKey) > 0 {
		authKey, err := token.AuthKeyFromFile(me.ApplePushSettings.ApplePushKey)
		if err != nil {
			LogCritical(fmt.Sprintf("Failed to load the apple push key err=%v for type=%v", err, me.ApplePushSettings.Type))
			return false
		}
		token := &token.Token{
			AuthKey: authKey,
			// KeyID from developer account (Certificates, Identifiers & Profiles -> Keys)
			KeyID: me.ApplePushSettings.AppleKeyId,
			// TeamID from developer account (View Account -> Membership)
			TeamID: me.ApplePushSettings.AppleTeamId,
		}
		if me.ApplePushSettings.ApplePushUseDevelopment {
			me.AppleClient = apns.NewTokenClient(token).Development()
		} else {
			me.AppleClient = apns.NewTokenClient(token).Production()
		}
		return true
	} else {
		LogError(fmt.Sprintf("Apple push notifications not configured.  Missing ApplePushCertPrivate. for type=%v", me.ApplePushSettings.Type))
		return false
	}
}

func (me *AppleNotificationServer) SendNotification(msg *PushNotification) PushResponse {
	notification := &apns.Notification{}
	notification.DeviceToken = msg.DeviceId
	payload := payload.NewPayload()
	notification.Payload = payload
	notification.Topic = me.ApplePushSettings.ApplePushTopic
	payload.Badge(msg.Badge)

	if msg.Type != PUSH_TYPE_CLEAR {
		payload.Alert(emoji.Sprint(msg.Message))
		payload.Category(msg.Category)
		payload.Sound("default")
	} else {
		payload.Alert("")
	}

	payload.Custom("type", msg.Type)

	if len(msg.ChannelId) > 0 {
		payload.Custom("channel_id", msg.ChannelId)
	}

	if len(msg.TeamId) > 0 {
		payload.Custom("team_id", msg.TeamId)
	}

	if len(msg.ChannelName) > 0 {
		payload.Custom("channel_name", msg.ChannelName)
	}

	if len(msg.SenderId) > 0 {
		payload.Custom("sender_id", msg.SenderId)
	}

	if len(msg.PostId) > 0 {
		payload.Custom("post_id", msg.PostId)
	}

	if len(msg.RootId) > 0 {
		payload.Custom("root_id", msg.RootId)
	}

	if len(msg.NewsId) > 0 {
		payload.Custom("news_id", msg.NewsId)
	}

	if len(msg.PushType) > 0 {
		payload.Custom("push_type", msg.PushType)
	}

	if len(msg.OverrideUsername) > 0 {
		payload.Custom("override_username", msg.OverrideUsername)
	}

	if len(msg.OverrideIconUrl) > 0 {
		payload.Custom("override_icon_url", msg.OverrideIconUrl)
	}

	if len(msg.FromWebhook) > 0 {
		payload.Custom("from_webhook", msg.FromWebhook)
	}

	if len(msg.PromoId) > 0 {
		payload.Custom("promo_id", msg.PromoId)
	}

	if me.AppleClient != nil {
		LogInfo(fmt.Sprintf("Sending apple push notification type=%v", me.ApplePushSettings.Type))
		start := time.Now()
		res, err := me.AppleClient.Push(notification)
		observeAPNSResponse(time.Since(start).Seconds())
		if err != nil {
			LogError(fmt.Sprintf("Failed to send apple push did=%v err=%v type=%v", msg.DeviceId, err, me.ApplePushSettings.Type))
			incrementFailure(me.ApplePushSettings.Type)
			return NewErrorPushResponse("unknown transport error")
		}

		if !res.Sent() {
			if res.Reason == "BadDeviceToken" || res.Reason == "Unregistered" || res.Reason == "MissingDeviceToken" || res.Reason == "DeviceTokenNotForTopic" {
				LogInfo(fmt.Sprintf("Failed to send apple push sending remove code res ApnsID=%v reason=%v code=%v type=%v", res.ApnsID, res.Reason, res.StatusCode, me.ApplePushSettings.Type))
				incrementRemoval(me.ApplePushSettings.Type)
				return NewRemovePushResponse()
			}

			LogError(fmt.Sprintf("Failed to send apple push with res ApnsID=%v reason=%v code=%v type=%v", res.ApnsID, res.Reason, res.StatusCode, me.ApplePushSettings.Type))
			incrementFailure(me.ApplePushSettings.Type)
			return NewErrorPushResponse("unknown send response error")
		}
	}

	incrementSuccess(me.ApplePushSettings.Type)
	return NewOkPushResponse()
}
