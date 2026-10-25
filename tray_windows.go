//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"

	"fyne.io/systray"
	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed assets/icon.ico
var iconICO []byte

// Размер мини-панели в «логических» пикселях (при 100% масштабе).
const popupW, popupH = 380, 600

func init() {
	// окно WebView2 и иконка трея должны жить в главном потоке ОС
	runtime.LockOSThread()
}

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	dwmapi                         = windows.NewLazySystemDLL("dwmapi.dll")
	pSetWindowLongPtr              = user32.NewProc("SetWindowLongPtrW")
	pSetWindowPos                  = user32.NewProc("SetWindowPos")
	pShowWindow                    = user32.NewProc("ShowWindow")
	pSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	pGetForegroundWindow           = user32.NewProc("GetForegroundWindow")
	pGetWindowThreadProcessId      = user32.NewProc("GetWindowThreadProcessId")
	pAttachThreadInput             = user32.NewProc("AttachThreadInput")
	pBringWindowToTop              = user32.NewProc("BringWindowToTop")
	pSendMessage                   = user32.NewProc("SendMessageW")
	pGetCursorPos                  = user32.NewProc("GetCursorPos")
	pMonitorFromPoint              = user32.NewProc("MonitorFromPoint")
	pGetMonitorInfo                = user32.NewProc("GetMonitorInfoW")
	pSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	pDwmSetWindowAttribute         = dwmapi.NewProc("DwmSetWindowAttribute")
	pGetDpiForMonitor              = windows.NewLazySystemDLL("shcore.dll").NewProc("GetDpiForMonitor")
)

const (
	gwlStyle        = ^uintptr(15) // -16
	gwlExStyle      = ^uintptr(19) // -20
	hwndTopmost     = ^uintptr(0)  // -1
	wsPopup         = 0x80000000
	wsClipChildren  = 0x02000000
	wsExToolWindow  = 0x00000080
	wsExTopmost     = 0x00000008
	swHide          = 0
	swpShowWindow   = 0x0040
	swpFrameChanged = 0x0020
	wmSize          = 0x0005
	dwmCornerPref   = 33
	dwmRound        = 2
	autostartKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	autostartName   = "DailyPlanner"
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }
type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type popup struct {
	wv       webview2.WebView
	hwnd     uintptr
	visible  atomic.Bool
	hiddenAt atomic.Int64
	shownAt  atomic.Int64
}

// runApp показывает иконку в трее и блокируется до выхода из программы.
func runApp(url string, s *Store, showPopup bool) {
	// чёткий текст на экранах с масштабом 125–200%
	if pSetProcessDpiAwarenessContext.Find() == nil {
		pSetProcessDpiAwarenessContext.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2 (-4)
	}

	p := newPopup(url)
	systray.Register(func() { onTrayReady(url, s, p) }, func() {
		if p != nil {
			p.wv.Terminate()
		}
	})
	if p != nil {
		if showPopup {
			// даём странице загрузиться, чтобы не мигнуть пустым окном
			time.AfterFunc(1200*time.Millisecond, func() { p.wv.Dispatch(p.show) })
		}
		p.wv.Run() // общий цикл сообщений для окна и иконки трея
		return
	}
	runMessageLoop()
}

func newPopup(url string) *popup {
	dataDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "DailyPlanner", "webview")
	wv := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  dataDir,
		AutoFocus: true,
		// окно создаётся видимым — делаем его 1×1, чтобы при запуске ничего не мигало;
		// настоящий размер выставляется при показе панели
		WindowOptions: webview2.WindowOptions{Title: "Ежедневник", Width: 1, Height: 1},
	})
	if wv == nil {
		log.Println("WebView2 недоступен — по клику на иконку будет открываться браузер")
		return nil
	}
	p := &popup{wv: wv, hwnd: uintptr(wv.Window())}
	pShowWindow.Call(p.hwnd, swHide)
	// окно без рамки, поверх всех, без кнопки на панели задач
	pSetWindowLongPtr.Call(p.hwnd, gwlStyle, wsPopup|wsClipChildren)
	pSetWindowLongPtr.Call(p.hwnd, gwlExStyle, wsExToolWindow|wsExTopmost)
	if pDwmSetWindowAttribute.Find() == nil {
		pref := uint32(dwmRound)
		pDwmSetWindowAttribute.Call(p.hwnd, dwmCornerPref, uintptr(unsafe.Pointer(&pref)), 4)
	}

	_ = wv.Bind("hidePopup", func() { p.hide() })
	_ = wv.Bind("openMain", func(view string) {
		p.hide()
		target := url
		if view != "" {
			target += "/#" + view
		}
		openBrowser(target)
	})
	_ = wv.Bind("quitApp", func() { systray.Quit() })
	wv.Navigate(url + "/popup.html")

	// прячем панель, когда она теряет фокус (клик мимо)
	go func() {
		for range time.Tick(120 * time.Millisecond) {
			if !p.visible.Load() || time.Since(time.UnixMilli(p.shownAt.Load())) < 400*time.Millisecond {
				continue
			}
			if fg, _, _ := pGetForegroundWindow.Call(); fg != p.hwnd {
				wv.Dispatch(p.hide)
			}
		}
	}()
	return p
}

func (p *popup) toggle() {
	if p.visible.Load() {
		p.hide()
		return
	}
	// клик по иконке сначала отбирает фокус у панели и прячет её — не открываем заново
	if time.Since(time.UnixMilli(p.hiddenAt.Load())) < 350*time.Millisecond {
		return
	}
	p.show()
}

func (p *popup) show() {
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	mon, _, _ := pMonitorFromPoint.Call(uintptr(*(*uint64)(unsafe.Pointer(&pt))), 2) // MONITOR_DEFAULTTONEAREST
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	pGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi)))

	// масштаб берём у монитора, на котором откроется панель
	scale := 1.0
	var dpiX, dpiY uint32
	if pGetDpiForMonitor.Find() == nil {
		if r, _, _ := pGetDpiForMonitor.Call(mon, 0, uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY))); r == 0 && dpiX > 0 {
			scale = float64(dpiX) / 96
		}
	}
	w, h, m := int32(popupW*scale), int32(popupH*scale), int32(12*scale)
	wa := mi.Work
	if h > wa.Bottom-wa.Top-2*m {
		h = wa.Bottom - wa.Top - 2*m
	}
	x := min(max(pt.X-w/2, wa.Left+m), wa.Right-w-m)
	y := wa.Bottom - h - m
	if pt.Y < wa.Top { // панель задач сверху
		y = wa.Top + m
	}
	pSetWindowPos.Call(p.hwnd, hwndTopmost, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpShowWindow|swpFrameChanged)
	// после смены стиля окна размер мог не измениться, и WebView не узнает о новой клиентской области
	pSendMessage.Call(p.hwnd, wmSize, 0, 0)
	forceForeground(p.hwnd)
	p.visible.Store(true)
	p.shownAt.Store(time.Now().UnixMilli())
	p.wv.Eval("window.onPopupShow && onPopupShow()")
}

// forceForeground делает окно активным. Windows разрешает это не всегда,
// поэтому при отказе временно подключаемся к очереди ввода активного окна.
func forceForeground(hwnd uintptr) {
	if r, _, _ := pSetForegroundWindow.Call(hwnd); r != 0 {
		return
	}
	fg, _, _ := pGetForegroundWindow.Call()
	fgThread, _, _ := pGetWindowThreadProcessId.Call(fg, 0)
	cur := uintptr(windows.GetCurrentThreadId())
	if fgThread != 0 && fgThread != cur {
		pAttachThreadInput.Call(cur, fgThread, 1)
		defer pAttachThreadInput.Call(cur, fgThread, 0)
	}
	pBringWindowToTop.Call(hwnd)
	pSetForegroundWindow.Call(hwnd)
}

func (p *popup) hide() {
	if !p.visible.Swap(false) {
		return
	}
	pShowWindow.Call(p.hwnd, swHide)
	p.hiddenAt.Store(time.Now().UnixMilli())
}

func onTrayReady(url string, s *Store, p *popup) {
	systray.SetIcon(iconICO)
	systray.SetTitle("Ежедневник")
	systray.SetTooltip("Ежедневник")
	if p != nil {
		systray.SetOnTapped(func() { p.wv.Dispatch(p.toggle) })
	} else {
		systray.SetOnTapped(func() { openBrowser(url) })
	}

	mOpen := systray.AddMenuItem("Открыть ежедневник", "Открыть в браузере")
	mAuto := systray.AddMenuItemCheckbox("Запускать вместе с Windows", "", autostartEnabled())
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Выход", "Закрыть программу")

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openBrowser(url)
			case <-mAuto.ClickedCh:
				if err := setAutostart(!mAuto.Checked()); err != nil {
					log.Printf("автозапуск: %v", err)
					continue
				}
				if autostartEnabled() {
					mAuto.Check()
				} else {
					mAuto.Uncheck()
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()

	// подсказка на иконке: сколько задач осталось на сегодня
	go func() {
		for {
			systray.SetTooltip(trayTooltip(s))
			time.Sleep(30 * time.Second)
		}
	}()
}

func trayTooltip(s *Store) string {
	today := time.Now().Format("2006-01-02")
	left, late := 0, 0
	for _, t := range s.Tasks() {
		switch {
		case t.Done || t.Due == "":
		case t.Due == today:
			left++
		case t.Due < today:
			late++
		}
	}
	tip := fmt.Sprintf("Ежедневник — на сегодня: %d", left)
	if late > 0 {
		tip += fmt.Sprintf(", просрочено: %d", late)
	}
	return tip
}

func autostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(autostartName)
	return err == nil
}

func setAutostart(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		return k.DeleteValue(autostartName)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(autostartName, `"`+exe+`" -autostart`)
}

// runMessageLoop нужен, только если WebView2 не запустился.
func runMessageLoop() {
	getMessage := user32.NewProc("GetMessageW")
	translate := user32.NewProc("TranslateMessage")
	dispatch := user32.NewProc("DispatchMessageW")
	var msg [48]byte
	for {
		r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		translate.Call(uintptr(unsafe.Pointer(&msg[0])))
		dispatch.Call(uintptr(unsafe.Pointer(&msg[0])))
	}
}
