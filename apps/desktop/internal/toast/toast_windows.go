//go:build windows

// Package toast shows a native Windows notification (an Action Center toast)
// for desktop events. WebView2 cannot deliver Web Push and its host denies the
// web Notification permission, so the desktop shell raises its order notices
// from Go instead. The toast is rendered by the Windows Runtime from a short
// PowerShell script, the mechanism the common Go toast libraries use.
package toast

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// appID is the source name Windows shows for the toasts in the Action Center.
const appID = "Palorosa Kitchen"

// createNoWindow keeps the helper PowerShell process from flashing a console.
const createNoWindow = 0x08000000

// Show displays a toast with the given title and body. It is best effort: the
// caller ignores the outcome, and the PowerShell process runs off the caller's
// goroutine so a slow start never blocks order handling.
func Show(title, body string) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" && body == "" {
		return
	}
	if title == "" {
		title, body = body, ""
	}
	go func() {
		_ = run(script(title, body))
	}()
}

// script builds the PowerShell that renders one ToastGeneric notification. The
// title and body travel base64 encoded so neither can break out of the script.
func script(title, body string) string {
	return fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$title = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s'))
$body = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s'))
$escapedTitle = [Security.SecurityElement]::Escape($title)
$escapedBody = [Security.SecurityElement]::Escape($body)

$xml = @"
<toast duration="short">
  <visual>
    <binding template="ToastGeneric">
      <text>$escapedTitle</text>
      <text>$escapedBody</text>
    </binding>
  </visual>
  <audio src="ms-winsoundevent:Notification.Default" />
</toast>
"@

$document = New-Object Windows.Data.Xml.Dom.XmlDocument
$document.LoadXml($xml)
$toast = New-Object Windows.UI.Notifications.ToastNotification $document
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('%s').Show($toast)
`, base64.StdEncoding.EncodeToString([]byte(title)), base64.StdEncoding.EncodeToString([]byte(body)), appID)
}

// run writes the script to a temporary file and executes it with the built-in
// PowerShell, hidden and without a profile.
func run(script string) error {
	file, err := os.CreateTemp("", "palorosa-toast-*.ps1")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	// The UTF-8 BOM makes Windows PowerShell read the script as UTF-8.
	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.WriteString(script); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	command := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", name)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return command.Run()
}
