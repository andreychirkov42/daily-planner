//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
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
	pIsZoomed                      = user32.NewProc("IsZoomed")
	pExtractIconEx                 = windows.NewLazySystemDLL("shell32.dll").NewProc("ExtractIconExW")
	pSHAppBarMessage               = windows.NewLazySystemDLL("shell32.dll").NewProc("SHAppBarMessage")
	pGetCursorPos                  = user32.NewProc("GetCursorPos")
	pMonitorFromPoint              = user32.NewProc("MonitorFromPoint")
	pGetMonitorInfo                = user32.NewProc("GetMonitorInfoW")
	pSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	pDwmSetWindowAttribute         = dwmapi.NewProc("DwmSetWindowAttribute")
	pGetDpiForMonitor              = windows.NewLazySystemDLL("shcore.dll").NewProc("GetDpiForMonitor")
)

const (
	gwlStyle          = ^uintptr(15) // -16
	gwlExStyle        = ^uintptr(19) // -20
	hwndTopmost       = ^uintptr(0)  // -1
	wsPopup           = 0x80000000
	wsClipChildren    = 0x02000000
	wsExToolWindow    = 0x00000080
	wsExTopmost       = 0x00000008
	swHide            = 0
	swpShowWindow     = 0x0040
	swpFrameChanged   = 0x0020
	wmSize            = 0x0005
	wmClose           = 0x0010
	wmSysCommand      = 0x0112
	wmNCLButtonDblClk = 0x00A3
	htSysMenu         = 3
	scMaximize        = 0xF030
	scRestore         = 0xF120
	wmSetIcon         = 0x0080
	gwlpWndProc       = ^uintptr(3) // -4
	swShow            = 5
	swRestore         = 9
	swpNoActivate     = 0x0010
	dwmCornerPref     = 33
	dwmUseDarkMode    = 20
	dwmBorderColor    = 34
	dwmCaptionColor   = 35
	dwmTextColor      = 36
	dwmRound          = 2
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
	appStore = o.store
	getAutostart = autostartEnabled
	putAutostart = func(on bool) error {
		err := setAutostart(on)
		syncAutostartMenu()
		return err
	}
	revealFile = func(path string) { _ = exec.Command("explorer", "/select,", path).Start() }
	// фон WebView2 до загрузки страницы — без белой вспышки при открытии окна (ARGB)
	if os.Getenv("WEBVIEW2_DEFAULT_BACKGROUND_COLOR") == "" {
		os.Setenv("WEBVIEW2_DEFAULT_BACKGROUND_COLOR", "FF0D121B")
	}
	setAppTheme = func(dark bool) {
		appDark.Store(dark)
		if h := mainHwnd; h != 0 {
			applyTitleBar(h, dark)
		}
	}
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
		WindowOptions: webview2.WindowOptions{Title: "Clepsy", Width: 1, Height: 1},
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
	// m — отступ от краёв экрана, gap — от панели задач (как у других панелей в трее)
	w, h, m, gap := int32(popupW*scale), int32(popupH*scale), int32(12*scale), int32(20*scale)
	wa := mi.Work
	// автоскрываемая панель задач не уменьшает рабочую область — берём её настоящее место
	if tb, ok := taskbarRect(); ok && tb.Left < mi.Monitor.Right && tb.Right > mi.Monitor.Left {
		if tb.Bottom >= mi.Monitor.Bottom-2 && tb.Top > mi.Monitor.Top && tb.Top < wa.Bottom {
			wa.Bottom = tb.Top // снизу
		} else if tb.Top <= mi.Monitor.Top+2 && tb.Bottom < mi.Monitor.Bottom && tb.Bottom > wa.Top {
			wa.Top = tb.Bottom // сверху
		}
	}
	if h > wa.Bottom-wa.Top-m-gap {
		h = wa.Bottom - wa.Top - m - gap
	}
	x := min(max(pt.X-w/2, wa.Left+m), wa.Right-w-m)
	y := wa.Bottom - h - gap
	if pt.Y < wa.Top { // панель задач сверху
		y = wa.Top + gap
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

// taskbarRect — где панель задач, когда она показана (в том числе автоскрываемая).
func taskbarRect() (rect, bool) {
	var abd struct {
		Size     uint32
		Hwnd     uintptr
		Callback uint32
		Edge     uint32
		Rc       rect
		LParam   uintptr
	}
	abd.Size = uint32(unsafe.Sizeof(abd))
	if r, _, _ := pSHAppBarMessage.Call(5, uintptr(unsafe.Pointer(&abd))); r == 0 { // ABM_GETTASKBARPOS
		return rect{}, false
	}
	return abd.Rc, true
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
		// по настройке: свернуть в трей (по умолчанию) или закрыть программу целиком
		if appStore != nil && appStore.AppSettings().CloseAction == "quit" {
			go systray.Quit()
		} else {
			pShowWindow.Call(hwnd, swHide)
		}
		return 0
	}
	// Двойной клик по значку в заголовке Windows по умолчанию закрывает окно;
	// значок стоит вплотную к названию, поэтому делаем как у заголовка — развернуть/вернуть.
	if msg == wmNCLButtonDblClk && wp == htSysMenu && hwnd == mainHwnd {
		cmd := uintptr(scMaximize)
		if r, _, _ := pIsZoomed.Call(hwnd); r != 0 {
			cmd = scRestore
		}
		pSendMessage.Call(hwnd, wmSysCommand, cmd, 0)
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
			WindowOptions: webview2.WindowOptions{Title: "Clepsy", Width: uint(1280 * scale), Height: uint(840 * scale), Center: true},
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
		applyTitleBar(mainHwnd, appDark.Load())
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

var appDark = func() *atomic.Bool { b := &atomic.Bool{}; b.Store(true); return b }()

// applyTitleBar красит системный заголовок окна в цвет приложения (Windows 11;
// на Windows 10 срабатывает только тёмный режим заголовка).
func applyTitleBar(hwnd uintptr, dark bool) {
	if pDwmSetWindowAttribute.Find() != nil {
		return
	}
	set := func(attr uintptr, v uint32) {
		pDwmSetWindowAttribute.Call(hwnd, attr, uintptr(unsafe.Pointer(&v)), 4)
	}
	// цвета в формате COLORREF: 0x00BBGGRR
	caption, text, border := uint32(0x001B120D), uint32(0x00F7EDE7), uint32(0x00443024) // #0d121b, #e7edf7, #243044
	mode := uint32(1)
	if !dark {
		caption, text, border, mode = 0x00FAF5F2, 0x002E2018, 0x00EFE6E0, 0 // #f2f5fa, #18202e, #e0e6ef
	}
	set(dwmUseDarkMode, mode)
	set(dwmCaptionColor, caption)
	set(dwmTextColor, text)
	set(dwmBorderColor, border)
}

var (
	appStore *Store            // для поведения крестика окна
	trayAuto *systray.MenuItem // пункт «Запускать вместе с Windows» в меню трея
)

// syncAutostartMenu ставит галочку в меню трея по фактическому состоянию автозапуска.
func syncAutostartMenu() {
	if trayAuto == nil {
		return
	}
	if autostartEnabled() {
		trayAuto.Check()
	} else {
		trayAuto.Uncheck()
	}
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
	systray.SetTitle("Clepsy")
	systray.SetTooltip("Clepsy")
	if a.pop != nil {
		systray.SetOnTapped(func() { a.pop.wv.Dispatch(a.pop.toggle) })
	} else {
		systray.SetOnTapped(func() { openBrowser(a.url) })
	}

	mOpen := systray.AddMenuItem("Открыть Clepsy", "Окно программы")
	mBrowser := systray.AddMenuItem("Открыть в браузере", "")
	mAuto := systray.AddMenuItemCheckbox("Запускать вместе с Windows", "", autostartEnabled())
	trayAuto = mAuto
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
				}
				syncAutostartMenu()
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
		name := map[string]string{"work": "Фокус", "short": "Перерыв", "long": "Длинный перерыв"}[st.Mode]
		return fmt.Sprintf("%s %02d:%02d · Clepsy", name, left/60, left%60)
	}
	left, late := s.DueCounts(today())
	tip := fmt.Sprintf("Clepsy — на сегодня: %d", left)
	if late > 0 {
		tip += fmt.Sprintf(", просрочено: %d", late)
	}
	return tip
}

// ---- автозапуск ----
// Два независимых способа, чтобы запуск при входе в Windows не зависел от одного механизма:
// запись в реестре Run и ярлык в папке «Автозагрузка». Если сработают оба, вторая копия
// увидит занятый порт и тихо закроется (флаг -autostart).

const (
	runKey        = `Software\Microsoft\Windows\CurrentVersion\Run`
	approvedRun   = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
	approvedLinks = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\StartupFolder`
	autostartName = "Clepsy"
	legacyName    = "DailyPlanner" // имя записи в прежних версиях
	startupLink   = "Clepsy.lnk"
)

// «включено» для StartupApproved; без этого флага Windows 11 может пропустить новую запись
var approvedOn = []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

func startupLinkPath() string {
	return filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs\Startup`, startupLink)
}

func autostartPresent() bool {
	return regString(runKey, autostartName) != "" || regString(runKey, legacyName) != ""
}

// autostartEnabled: запись в Run есть, и ни её, ни ярлык не выключили в «Автозагрузке» диспетчера задач.
func autostartEnabled() bool {
	return autostartPresent() &&
		!disabledInTaskManager(approvedRun, autostartName) &&
		!disabledInTaskManager(approvedRun, legacyName) &&
		!disabledInTaskManager(approvedLinks, startupLink)
}

// «выключено» для StartupApproved
var approvedOff = []byte{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

func disabledInTaskManager(key, name string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	flag, _, err := k.GetBinaryValue(name)
	return err == nil && len(flag) > 0 && flag[0]&1 == 1 // нечётный первый байт — «отключено»
}

func regString(key, name string) string {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue(name)
	return v
}

func setRegValue(key, name string, value any) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	switch v := value.(type) {
	case nil:
		if err := k.DeleteValue(name); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	case string:
		return k.SetStringValue(name, v)
	case []byte:
		return k.SetBinaryValue(name, v)
	}
	return nil
}

func setAutostart(on bool) error {
	// старое имя записи убираем в любом случае
	_ = setRegValue(runKey, legacyName, nil)
	_ = setRegValue(approvedRun, legacyName, nil)
	if !on {
		_ = setRegValue(approvedRun, autostartName, nil)
		_ = setRegValue(approvedLinks, startupLink, nil)
		_ = os.Remove(startupLinkPath())
		return setRegValue(runKey, autostartName, nil)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := setRegValue(runKey, autostartName, `"`+exe+`" -autostart`); err != nil {
		return err
	}
	if err := setRegValue(approvedRun, autostartName, approvedOn); err != nil {
		return err
	}
	if err := writeStartupLink(exe); err != nil {
		log.Printf("ярлык автозагрузки: %v", err) // запись в Run уже есть — не критично
		return nil
	}
	return setRegValue(approvedLinks, startupLink, approvedOn)
}

// writeStartupLink создаёт ярлык в «Автозагрузке» через WScript.Shell (без окна консоли).
func writeStartupLink(exe string) error {
	ps := fmt.Sprintf(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut('%s');$s.TargetPath='%s';$s.Arguments='-autostart';$s.WorkingDirectory='%s';$s.Description='Clepsy';$s.Save()`,
		psQuote(startupLinkPath()), psQuote(exe), psQuote(filepath.Dir(exe)))
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

// repairAutostart при каждом старте: переносит старую запись на новое имя, чинит путь,
// если exe перенесли, и восстанавливает ярлык. Выключенный пользователем автозапуск не трогает.
func repairAutostart() {
	if !autostartEnabled() {
		if autostartPresent() {
			// одну из двух записей выключили в диспетчере задач — выключаем и вторую,
			// иначе программа всё равно запустится через неё
			_ = setRegValue(approvedRun, autostartName, approvedOff)
			if _, err := os.Stat(startupLinkPath()); err == nil {
				_ = setRegValue(approvedLinks, startupLink, approvedOff)
			}
		}
		return
	}
	exe, _ := os.Executable()
	want := `"` + exe + `" -autostart`
	_, linkErr := os.Stat(startupLinkPath())
	if regString(runKey, autostartName) == want && regString(runKey, legacyName) == "" && linkErr == nil {
		return // всё уже на месте — не запускаем PowerShell зря
	}
	if err := setAutostart(true); err != nil {
		log.Printf("автозапуск: %v", err)
	}
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
