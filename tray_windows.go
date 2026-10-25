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
const popupW, popupH = 380, 680

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
	pCallWindowProc                = user32.NewProc("CallWindowProcW")
	pIsIconic                      = user32.NewProc("IsIconic")
	pExtractIconEx                 = windows.NewLazySystemDLL("shell32.dll").NewProc("ExtractIconExW")
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
	wmClose         = 0x0010
	wmSetIcon       = 0x0080
	gwlpWndProc     = ^uintptr(3) // -4
	swShow          = 5
	swRestore       = 9
	swpNoActivate   = 0x0010
	dwmCornerPref   = 33
	dwmRound        = 2
	autostartKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	autostartName   = "DailyPlanner"
	approvedKey     = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
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
	wv        webview2.WebView
	hwnd      uintptr
	visible   atomic.Bool
	passive   atomic.Bool // показана сама (сигнал таймера), без перехвата фокуса
	activated atomic.Bool
	hiddenAt  atomic.Int64
	shownAt   atomic.Int64
}

// app связывает иконку в трее, мини-панель и окно ежедневника.
type app struct {
	url   string
	store *Store
	pomo  *Pomodoro
	pop   *popup
	main  webview2.WebView
}

// runApp показывает иконку в трее и блокируется до выхода из программы.
func runApp(o appOptions) {
	// чёткий текст на экранах с масштабом 125–200%
	if pSetProcessDpiAwarenessContext.Find() == nil {
		pSetProcessDpiAwarenessContext.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2 (-4)
	}

	repairAutostart()
	a := &app{url: o.url, store: o.store, pomo: o.pomo}
	a.pop = newPopup(a)
	if a.pop != nil {
		// все обращения к окнам — только из главного потока, через очередь панели
		openAppWindow = func(view string) bool {
			a.pop.wv.Dispatch(func() { a.showMain(view) })
			return true
		}
		o.pomo.onFinish = func(_, _ string) { a.pop.wv.Dispatch(a.pop.showAtTray) }
	}
	systray.Register(a.onTrayReady, func() {
		if a.pop != nil {
			a.pop.wv.Terminate()
		}
	})
	if a.pop == nil {
		if o.showMain {
			openBrowser(o.url)
		}
		runMessageLoop()
		return
	}
	if o.showPopup {
		// даём странице загрузиться, чтобы не мигнуть пустым окном
		time.AfterFunc(1200*time.Millisecond, func() { a.pop.wv.Dispatch(a.pop.show) })
	}
	if o.showMain {
		time.AfterFunc(200*time.Millisecond, func() { a.pop.wv.Dispatch(func() { a.showMain("") }) })
	}
	a.pop.wv.Run() // общий цикл сообщений для окон и иконки трея
}

func webviewDataDir() string {
	// для тестов: отдельная папка, чтобы не мешать запущенной копии
	if dir := os.Getenv("DAILYPLANNER_WEBVIEW_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "DailyPlanner", "webview")
}

func newPopup(a *app) *popup {
	wv := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  webviewDataDir(),
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
	// Окно создаём не прямо в обработчике нажатия: WebView2 не умеет создавать новое окно
	// изнутри события другого — откладываем в очередь главного потока.
	_ = wv.Bind("openMain", func(view string) { wv.Dispatch(func() { a.showMain(view) }) })
	_ = wv.Bind("openInBrowser", func(view string) {
		p.hide()
		openBrowser(a.url + "/#" + view)
	})
	_ = wv.Bind("quitApp", func() { systray.Quit() })
	wv.Navigate(a.url + "/popup.html")

	// прячем панель, когда она теряет фокус (клик мимо)
	go func() {
		for range time.Tick(120 * time.Millisecond) {
			if !p.visible.Load() {
				continue
			}
			since := time.Since(time.UnixMilli(p.shownAt.Load()))
			fg, _, _ := pGetForegroundWindow.Call()
			switch {
			case fg == p.hwnd:
				p.activated.Store(true)
			case p.passive.Load() && !p.activated.Load():
				// панель от таймера висит 20 секунд, пока её не тронут
				if since > 20*time.Second {
					wv.Dispatch(p.hide)
				}
			case since > 400*time.Millisecond:
				wv.Dispatch(p.hide)
			}
		}
	}()
	return p
}

func (p *popup) toggle() {
	if p.visible.Load() && !p.passive.Load() {
		p.hide()
		return
	}
	// клик по иконке сначала отбирает фокус у панели и прячет её — не открываем заново
	if time.Since(time.UnixMilli(p.hiddenAt.Load())) < 350*time.Millisecond {
		return
	}
	p.show()
}

// show открывает панель у курсора (клик по иконке в трее) и делает её активной.
func (p *popup) show() {
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	p.place(pt, false)
}

// showAtTray показывает панель у края основного монитора, не отбирая фокус (сигнал таймера).
func (p *popup) showAtTray() {
	if p.visible.Load() {
		p.wv.Eval("window.onPopupShow && onPopupShow()")
		return
	}
	mon, _, _ := pMonitorFromPoint.Call(0, 1) // MONITOR_DEFAULTTOPRIMARY
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	pGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi)))
	p.place(point{mi.Work.Right - 1, mi.Work.Bottom - 1}, true)
}

func (p *popup) place(pt point, passive bool) {
	mon, _, _ := pMonitorFromPoint.Call(uintptr(*(*uint64)(unsafe.Pointer(&pt))), 2) // MONITOR_DEFAULTTONEAREST
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	pGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi)))

	scale := monitorScale(mon)
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
	flags := uintptr(swpShowWindow | swpFrameChanged)
	if passive {
		flags |= swpNoActivate
	}
	pSetWindowPos.Call(p.hwnd, hwndTopmost, uintptr(x), uintptr(y), uintptr(w), uintptr(h), flags)
	// после смены стиля окна размер мог не измениться, и WebView не узнает о новой клиентской области
	pSendMessage.Call(p.hwnd, wmSize, 0, 0)
	p.passive.Store(passive)
	p.activated.Store(false)
	if !passive {
		forceForeground(p.hwnd)
	}
	p.visible.Store(true)
	p.shownAt.Store(time.Now().UnixMilli())
	p.wv.Eval("window.onPopupShow && onPopupShow()")
}

func monitorScale(mon uintptr) float64 {
	var dpiX, dpiY uint32
	if pGetDpiForMonitor.Find() == nil {
		if r, _, _ := pGetDpiForMonitor.Call(mon, 0, uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY))); r == 0 && dpiX > 0 {
			return float64(dpiX) / 96
		}
	}
	return 1
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

// ---- окно ежедневника: одно на всё приложение, вместо новых вкладок браузера ----

var (
	mainHwnd    uintptr
	mainOldProc uintptr
	mainProc    = windows.NewCallback(mainWndProc)
)

// mainWndProc прячет окно по крестику вместо закрытия: иначе завершилась бы вся программа.
func mainWndProc(hwnd, msg, wp, lp uintptr) uintptr {
	if msg == wmClose && hwnd == mainHwnd {
		pShowWindow.Call(hwnd, swHide)
		return 0
	}
	r, _, _ := pCallWindowProc.Call(mainOldProc, hwnd, msg, wp, lp)
	return r
}

func (a *app) showMain(view string) {
	a.pop.hide()
	if a.main == nil {
		var pt point
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		mon, _, _ := pMonitorFromPoint.Call(uintptr(*(*uint64)(unsafe.Pointer(&pt))), 2)
		scale := monitorScale(mon)
		wv := webview2.NewWithOptions(webview2.WebViewOptions{
			DataPath:      webviewDataDir(),
			AutoFocus:     true,
			WindowOptions: webview2.WindowOptions{Title: "Ежедневник", Width: uint(1280 * scale), Height: uint(840 * scale), Center: true},
		})
		if wv == nil {
			openBrowser(a.url + "/#" + view)
			return
		}
		a.main = wv
		mainHwnd = uintptr(wv.Window())
		mainOldProc, _, _ = pSetWindowLongPtr.Call(mainHwnd, gwlpWndProc, mainProc)
		wv.SetSize(int(820*scale), int(560*scale), webview2.HintMin)
		setWindowIcon(mainHwnd)
		wv.Navigate(a.url + "/#" + view)
	} else if view != "" {
		a.main.Eval(fmt.Sprintf("window.openView && openView(%q)", view))
	}
	if r, _, _ := pIsIconic.Call(mainHwnd); r != 0 {
		pShowWindow.Call(mainHwnd, swRestore)
	} else {
		pShowWindow.Call(mainHwnd, swShow)
	}
	forceForeground(mainHwnd)
}

// setWindowIcon ставит окну иконку из ресурсов exe (заголовок и панель задач).
func setWindowIcon(hwnd uintptr) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	path, _ := windows.UTF16PtrFromString(exe)
	var large, small uintptr
	pExtractIconEx.Call(uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&large)), uintptr(unsafe.Pointer(&small)), 1)
	if large != 0 {
		pSendMessage.Call(hwnd, wmSetIcon, 1, large)
	}
	if small != 0 {
		pSendMessage.Call(hwnd, wmSetIcon, 0, small)
	}
}

func (a *app) onTrayReady() {
	systray.SetIcon(iconICO)
	systray.SetTitle("Ежедневник")
	systray.SetTooltip("Ежедневник")
	if a.pop != nil {
		systray.SetOnTapped(func() { a.pop.wv.Dispatch(a.pop.toggle) })
	} else {
		systray.SetOnTapped(func() { openBrowser(a.url) })
	}

	mOpen := systray.AddMenuItem("Открыть ежедневник", "Окно программы")
	mBrowser := systray.AddMenuItem("Открыть в браузере", "")
	mAuto := systray.AddMenuItemCheckbox("Запускать вместе с Windows", "", autostartEnabled())
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Выход", "Закрыть программу")

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				if openAppWindow == nil || !openAppWindow("") {
					openBrowser(a.url)
				}
			case <-mBrowser.ClickedCh:
				openBrowser(a.url)
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

	// подсказка на иконке: таймер, если идёт, иначе задачи на сегодня
	go func() {
		last := ""
		for range time.Tick(time.Second) {
			if tip := trayTooltip(a.store, a.pomo); tip != last {
				systray.SetTooltip(tip)
				last = tip
			}
		}
	}()
}

func trayTooltip(s *Store, pomo *Pomodoro) string {
	if st := pomo.State(); st.Running {
		left := max(0, int((st.EndAt-time.Now().UnixMilli()+999)/1000))
		name := map[string]string{"work": "🍅 Фокус", "short": "☕ Перерыв", "long": "🌿 Длинный перерыв"}[st.Mode]
		return fmt.Sprintf("%s %02d:%02d · Ежедневник", name, left/60, left%60)
	}
	today := today()
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

// autostartEnabled учитывает и запись в Run, и флаг из «Автозагрузки» диспетчера задач.
func autostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	if _, _, err = k.GetStringValue(autostartName); err != nil {
		return false
	}
	a, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer a.Close()
	flag, _, err := a.GetBinaryValue(autostartName)
	return err != nil || len(flag) == 0 || flag[0]&1 == 0 // нечётный первый байт — «отключено»
}

func setAutostart(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		_ = setApproved(nil)
		if err := k.DeleteValue(autostartName); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := k.SetStringValue(autostartName, `"`+exe+`" -autostart`); err != nil {
		return err
	}
	// Без явного «включено» в StartupApproved Windows 11 может пропустить новую запись в Run.
	return setApproved([]byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
}

// setApproved пишет флаг автозагрузки (nil — удалить запись).
func setApproved(flag []byte) error {
	a, _, err := registry.CreateKey(registry.CURRENT_USER, approvedKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer a.Close()
	if flag == nil {
		return a.DeleteValue(autostartName)
	}
	return a.SetBinaryValue(autostartName, flag)
}

// repairAutostart чинит запись, если exe перенесли в другую папку или флаг одобрения потерялся.
func repairAutostart() {
	if !autostartEnabled() {
		return
	}
	_ = setAutostart(true)
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
