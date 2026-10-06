//go:build windows

package window

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// webViewClient is the EdgeUpdate client id of the Microsoft Edge WebView2
// Evergreen runtime.
const webViewClient = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

// WebViewDownloadURL is the official Evergreen bootstrapper Microsoft
// publishes (it downloads and installs the runtime itself).
const WebViewDownloadURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

// idYes is the MessageBox return for the Yes button (winuser.h).
const idYes = 6

// maxInstallerSize caps the bootstrapper download.
const maxInstallerSize = 64 << 20

// Available reports whether the WebView2 runtime is installed. The embedded
// engine checks the same thing internally, but when it is missing it hands
// back a null handle and the first call would crash the process, so main asks
// first. KITCHEN_NO_WEBVIEW=1 forces the missing path, for testing.
func Available() bool {
	if strings.TrimSpace(os.Getenv("KITCHEN_NO_WEBVIEW")) == "1" {
		return false
	}
	return installed()
}

// installed is the real registry probe. 64-bit Windows registers the runtime
// under WOW6432Node; the plain key covers 32-bit systems and per-user
// installs.
func installed() bool {
	probes := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\` + webViewClient},
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\EdgeUpdate\Clients\` + webViewClient},
		{registry.CURRENT_USER, `Software\Microsoft\EdgeUpdate\Clients\` + webViewClient},
	}
	for _, probe := range probes {
		key, err := registry.OpenKey(probe.root, probe.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		version, _, err := key.GetStringValue("pv")
		key.Close()
		if err == nil && version != "" && version != "0.0.0.0" {
			return true
		}
	}
	return false
}

// AskInstallWebView asks whether to download and install the runtime now.
func AskInstallWebView() bool {
	text, _ := windows.UTF16PtrFromString("Falta el componente WebView2 de Microsoft y la aplicación no puede mostrar la ventana.\n\n" +
		"¿Descargar e instalar ahora el runtime oficial? Es silencioso y puede tardar un poco.")
	caption, _ := windows.UTF16PtrFromString("Palorosa — falta WebView2")
	result, _ := windows.MessageBox(0, text, caption, windows.MB_YESNO|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
	return result == idYes
}

// AskOpenWebViewPage asks whether to open the download page in the browser.
func AskOpenWebViewPage() bool {
	text, _ := windows.UTF16PtrFromString("No se pudo instalar WebView2 automáticamente.\n\n" +
		"¿Abrir la página de descarga en el navegador?")
	caption, _ := windows.UTF16PtrFromString("Palorosa — falta WebView2")
	result, _ := windows.MessageBox(0, text, caption, windows.MB_YESNO|windows.MB_ICONWARNING|windows.MB_SETFOREGROUND)
	return result == idYes
}

// InstallWebView downloads the official Evergreen bootstrapper and runs it
// silently. It reports an error when the runtime is still missing afterwards,
// which happens when the install needs a permission the user does not have.
func InstallWebView() error {
	client := &http.Client{Timeout: 15 * time.Minute}
	response, err := client.Get(WebViewDownloadURL)
	if err != nil {
		return fmt.Errorf("no se pudo descargar el instalador: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("la descarga del instalador respondió %s", response.Status)
	}
	target := filepath.Join(os.TempDir(), "palorosa-webview2-setup.exe")
	file, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("no se pudo guardar el instalador: %w", err)
	}
	_, copyErr := io.Copy(file, io.LimitReader(response.Body, maxInstallerSize))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return fmt.Errorf("no se pudo guardar el instalador: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(target)
		return fmt.Errorf("no se pudo guardar el instalador: %w", closeErr)
	}
	command := exec.Command(target, "/silent", "/install")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	runErr := command.Run()
	_ = os.Remove(target)
	if runErr != nil {
		return fmt.Errorf("el instalador terminó con error: %w", runErr)
	}
	if !installed() {
		return errors.New("el instalador terminó pero el runtime no quedó disponible; puede requerir permisos de administrador")
	}
	return nil
}
