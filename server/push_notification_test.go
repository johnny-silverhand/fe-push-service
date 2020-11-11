package server

import (
	"strings"
	"testing"
)

func TestPushNotification(t *testing.T) {
	msg := PushNotification{Platform: "test"}
	json := msg.ToJson()
	result := PushNotificationFromJson(strings.NewReader(json))

	if msg.Platform != result.Platform {
		t.Fatal("Ids do not match")
	}
}
