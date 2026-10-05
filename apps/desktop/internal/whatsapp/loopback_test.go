package whatsapp

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func selfInfo() types.MessageInfo {
	jid := types.NewJID("573001112233", types.DefaultUserServer)
	return types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     jid,
			Sender:   jid,
			IsFromMe: true,
		},
	}
}

func otherInfo(sender string) types.MessageInfo {
	return types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:   types.NewJID(sender, types.DefaultUserServer),
			Sender: types.NewJID(sender, types.DefaultUserServer),
		},
	}
}

func TestAcceptedMessages(t *testing.T) {
	tests := []struct {
		name     string
		loopback bool
		info     types.MessageInfo
		want     bool
	}{
		{
			name: "allowlisted number is accepted",
			info: otherInfo("573001112233"),
			want: true,
		},
		{
			name: "unknown number is ignored",
			info: otherInfo("573999999999"),
		},
		{
			name: "self-chat is ignored without loopback",
			info: selfInfo(),
		},
		{
			name:     "self-chat is accepted with loopback",
			loopback: true,
			info:     selfInfo(),
			want:     true,
		},
		{
			name:     "a message from me to someone else stays ignored",
			loopback: true,
			info: func() types.MessageInfo {
				info := selfInfo()
				info.Chat = types.NewJID("573999999999", types.DefaultUserServer)
				return info
			}(),
		},
		{
			name:     "group messages are ignored",
			loopback: true,
			info: func() types.MessageInfo {
				info := otherInfo("573001112233")
				info.IsGroup = true
				return info
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &Client{cfg: Config{Loopback: test.loopback, Allowlist: []string{"573001112233"}}}
			if got := client.accepted(test.info); got != test.want {
				t.Fatalf("accepted = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSelfChatAcrossLID(t *testing.T) {
	// WhatsApp may address a self-chat by LID; sender and chat stay the same user.
	lid := types.NewJID("123456789012345", types.HiddenUserServer)
	info := types.MessageInfo{
		MessageSource: types.MessageSource{Chat: lid, Sender: lid, IsFromMe: true},
	}
	if !isSelfChat(info) {
		t.Fatal("a LID self-chat must be detected")
	}

	info.Chat = types.NewJID("999999999999", types.HiddenUserServer)
	if isSelfChat(info) {
		t.Fatal("two different users are not a self-chat")
	}
}

func TestOwnSentMessagesAreNotCommands(t *testing.T) {
	client := &Client{cfg: Config{Loopback: true}, sent: map[types.MessageID]time.Time{}}
	info := selfInfo()
	info.ID = "REPLY-1"
	client.rememberSent(info.ID)
	if client.accepted(info) {
		t.Fatal("the echo of a message this account sent must be ignored")
	}

	info.ID = "COMMAND-1"
	if !client.accepted(info) {
		t.Fatal("a fresh self-chat command must be accepted")
	}
}
