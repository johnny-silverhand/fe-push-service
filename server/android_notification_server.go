// Copyright (c) 2015 Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"fmt"
	"time"

	fcm "github.com/NaySoftware/go-fcm"
	"github.com/kyokomi/emoji"

)


type Message struct {
	RegistrationIDs       []string               `json:"registration_ids"`
	CollapseKey           string                 `json:"collapse_key,omitempty"`
	Data                  map[string]interface{} `json:"data,omitempty"`
	DelayWhileIdle        bool                   `json:"delay_while_idle,omitempty"`
	TimeToLive            int                    `json:"time_to_live,omitempty"`
	RestrictedPackageName string                 `json:"restricted_package_name,omitempty"`
	DryRun                bool                   `json:"dry_run,omitempty"`
}

// NewMessage returns a new Message with the specified payload
// and registration IDs.
func NewMessage(data map[string]interface{}, regIDs ...string) *Message {
	return &Message{RegistrationIDs: regIDs, Data: data}
}


type AndroidNotificationServer struct {
	AndroidPushSettings AndroidPushSettings
}

func NewAndroideNotificationServer(settings AndroidPushSettings) NotificationServer {
	return &AndroidNotificationServer{AndroidPushSettings: settings}
}

func (me *AndroidNotificationServer) Initialize() bool {
	LogInfo(fmt.Sprintf("Initializing Android notificaiton server for type=%v", me.AndroidPushSettings.Type))

	if len(me.AndroidPushSettings.AndroidApiKey) == 0 {
		LogError("Android push notifications not configured.  Mssing AndroidApiKey.")
		return false
	}

	return true
}

func (me *AndroidNotificationServer) SendNotification(msg *PushNotification) PushResponse {
	var data map[string]interface{}
	if msg.Type == PUSH_TYPE_CLEAR {
		data = map[string]interface{}{
			"type":              PUSH_TYPE_CLEAR,
			"badge":             msg.Badge,
			"channel_id":        msg.ChannelId,
			"team_id":           msg.TeamId,
			"sender_id":         msg.SenderId,
			"override_username": msg.OverrideUsername,
			"override_icon_url": msg.OverrideIconUrl,
			"from_webhook":      msg.FromWebhook,
		}
	} else {
		data = map[string]interface{}{
			"type":              PUSH_TYPE_MESSAGE,
			"badge":             msg.Badge,
			"message":           emoji.Sprint(msg.Message),
			"channel_id":        msg.ChannelId,
			"channel_name":      msg.ChannelName,
			"team_id":           msg.TeamId,
			"post_id":           msg.PostId,
			"root_id":           msg.RootId,
			"sender_id":         msg.SenderId,
			"override_username": msg.OverrideUsername,
			"override_icon_url": msg.OverrideIconUrl,
			"from_webhook":      msg.FromWebhook,
		}
	}

	regIDs := []string{msg.DeviceId}
	//gcmMsg := NewMessage(data, regIDs...)

	sender := fcm.NewFcmClient(me.AndroidPushSettings.AndroidApiKey)
	//sender.NewFcmMsgTo("", data)
	notification := &fcm.NotificationPayload{}
	sender.NewFcmRegIdsMsg(regIDs, data)
	notification.Title = msg.Message
	notification.Icon = "ic_launcher"
	notification.Body = emoji.Sprint(msg.Message)

	sender.SetNotificationPayload(notification)

	//sender.AppendDevices(xds)
	/*sender := &gcm.Sender{
		ApiKey: me.AndroidPushSettings.AndroidApiKey,
		Http:   httpClient,
	}*/

	if len(me.AndroidPushSettings.AndroidApiKey) > 0 {
		LogInfo(fmt.Sprintf("Sending android push notification for type=%v", me.AndroidPushSettings.Type))
		start := time.Now()
		resp, err := sender.Send()
		observeGCMResponse(time.Since(start).Seconds())

		if err != nil {
			LogError(fmt.Sprintf("Failed to send GCM push did=%v err=%v type=%v", msg.DeviceId, err, me.AndroidPushSettings.Type))
			incrementFailure(me.AndroidPushSettings.Type)
			return NewErrorPushResponse("unknown transport error")
		}

		if resp.Fail > 0 {
			if len(resp.Results) > 0 && (resp.Results[0]["error"] == "InvalidRegistration" || resp.Results[0]["error"] == "NotRegistered") {
				LogInfo(fmt.Sprintf("Android response failure sending remove code: %v type=%v", resp, me.AndroidPushSettings.Type))
				incrementRemoval(me.AndroidPushSettings.Type)
				return NewRemovePushResponse()
			}
			LogError(fmt.Sprintf("Android response failure: %v type=%v", resp, me.AndroidPushSettings.Type))
			incrementFailure(me.AndroidPushSettings.Type)
			return NewErrorPushResponse("unknown send response error")
		}

		//LogError(fmt.Sprintf("Android resp: %v type=%v", resp, resp.Results))
	}

	incrementSuccess(me.AndroidPushSettings.Type)
	return NewOkPushResponse()
}
