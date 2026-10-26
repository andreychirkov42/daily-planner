package io.github.andreychirkov42.clepsy;

import android.app.Activity;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.util.Log;
import android.view.View;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.webkit.CookieManager;
import android.webkit.JavascriptInterface;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import android.window.OnBackInvokedDispatcher;

import java.io.File;
import java.net.HttpURLConnection;
import java.net.URL;
import java.security.SecureRandom;
import java.util.Map;
import java.util.TimeZone;

/**
 * Оболочка Clepsy: запускает встроенный Go-сервер (libclepsy.so) и показывает его страницы в WebView.
 * Вся логика задач — в сервере, общем с версией для Windows.
 */
public class MainActivity extends Activity {
    static final int PORT = 18080;
    static final String HOME = "http://127.0.0.1:" + PORT + "/";
    static final int PICK_FILE = 1;
    static final String TAG = "Clepsy";

    // сервер живёт, пока жив процесс приложения; при пересоздании окна запускать его заново не нужно
    private static Process server;

    private FrameLayout root;
    private WebView web;
    private ValueCallback<Uri[]> fileCallback;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        if (Build.VERSION.SDK_INT >= 30) getWindow().setDecorFitsSystemWindows(false);

        web = new WebView(this);
        WebSettings ws = web.getSettings();
        ws.setJavaScriptEnabled(true);
        ws.setDomStorageEnabled(true);
        web.setBackgroundColor(Color.TRANSPARENT);
        web.addJavascriptInterface(new Bridge(), "ClepsyAndroid");
        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest req) {
                Uri u = req.getUrl();
                if ("127.0.0.1".equals(u.getHost())) return false;
                openExternal(u); // чужие ссылки — во внешнем браузере
                return true;
            }
        });
        web.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> cb, FileChooserParams params) {
                if (fileCallback != null) fileCallback.onReceiveValue(null);
                fileCallback = cb;
                try {
                    startActivityForResult(params.createIntent(), PICK_FILE);
                } catch (Exception e) {
                    fileCallback = null;
                    return false;
                }
                return true;
            }
        });

        root = new FrameLayout(this);
        root.addView(web);
        // страница рисуется от края до края: отступы под строку состояния, жесты и клавиатуру
        root.setOnApplyWindowInsetsListener((v, insets) -> {
            if (Build.VERSION.SDK_INT >= 30) {
                android.graphics.Insets bars = insets.getInsets(WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
                android.graphics.Insets ime = insets.getInsets(WindowInsets.Type.ime());
                v.setPadding(bars.left, bars.top, bars.right, Math.max(bars.bottom, ime.bottom));
            }
            return insets;
        });
        setContentView(root);

        if (Build.VERSION.SDK_INT >= 33) {
            getOnBackInvokedDispatcher().registerOnBackInvokedCallback(
                    OnBackInvokedDispatcher.PRIORITY_DEFAULT, this::goBack);
        }

        String token = token();
        CookieManager cm = CookieManager.getInstance();
        cm.setCookie(HOME, "clepsy=" + token + "; Path=/; HttpOnly; SameSite=Strict");
        cm.flush();

        new Thread(() -> {
            boolean ok = startServer(token);
            runOnUiThread(() -> {
                if (ok) web.loadUrl(HOME);
                else web.loadDataWithBaseURL(null, "<body style='font:16px sans-serif;color:#e7edf7;padding:24px'>"
                        + "Не удалось запустить Clepsy. Попробуйте закрыть приложение и открыть снова.</body>",
                        "text/html", "utf-8", null);
            });
        }).start();
    }

    /** Запускает Go-сервер, если он ещё не отвечает, и ждёт его готовности. */
    private boolean startServer(String token) {
        if (alive()) return true;
        try {
            File exe = new File(getApplicationInfo().nativeLibraryDir, "libclepsy.so");
            ProcessBuilder pb = new ProcessBuilder(exe.getPath(), "-no-tray", "-no-open",
                    "-data", new File(getFilesDir(), "data.json").getPath(),
                    "-addr", "127.0.0.1:" + PORT);
            Map<String, String> env = pb.environment();
            env.put("TZ", TimeZone.getDefault().getID());
            env.put("TMPDIR", getCacheDir().getPath());
            env.put("CLEPSY_TOKEN", token);
            pb.redirectErrorStream(true);
            pb.redirectOutput(new File(getCacheDir(), "server.out"));
            server = pb.start();
            for (int i = 0; i < 100; i++) {
                if (alive()) return true;
                if (!server.isAlive()) break;
                Thread.sleep(100);
            }
        } catch (Exception e) {
            Log.e(TAG, "запуск сервера", e);
        }
        return false;
    }

    private static boolean alive() {
        try {
            HttpURLConnection c = (HttpURLConnection) new URL(HOME).openConnection();
            c.setConnectTimeout(300);
            c.setReadTimeout(1000);
            int code = c.getResponseCode();
            c.disconnect();
            return code == 200;
        } catch (Exception e) {
            return false;
        }
    }

    /** Код доступа к API: постоянный, чтобы уже запущенный сервер принимал cookie нового окна. */
    private String token() {
        SharedPreferences p = getSharedPreferences("clepsy", MODE_PRIVATE);
        String t = p.getString("token", null);
        if (t == null) {
            byte[] b = new byte[24];
            new SecureRandom().nextBytes(b);
            StringBuilder sb = new StringBuilder();
            for (byte x : b) sb.append(String.format("%02x", x));
            t = sb.toString();
            p.edit().putString("token", t).apply();
        }
        return t;
    }

    private void goBack() {
        if (web.canGoBack()) web.goBack();
        else moveTaskToBack(true); // не закрываем: сервер и таймер Помодоро продолжают работать
    }

    @Override
    @SuppressWarnings("deprecation")
    public void onBackPressed() {
        goBack(); // до Android 13
    }

    @Override
    protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        if (request == PICK_FILE && fileCallback != null) {
            fileCallback.onReceiveValue(WebChromeClient.FileChooserParams.parseResult(result, data));
            fileCallback = null;
        }
    }

    private void openExternal(Uri u) {
        if (!"https".equals(u.getScheme())) return;
        try {
            startActivity(new Intent(Intent.ACTION_VIEW, u));
        } catch (Exception e) {
            Log.w(TAG, "нет приложения для " + u, e);
        }
    }

    /** Строка состояния и полоса навигации в цвет темы страницы. */
    @SuppressWarnings("deprecation")
    private void applyBars(boolean dark, int bg) {
        root.setBackgroundColor(bg);
        getWindow().getDecorView().setBackgroundColor(bg);
        if (Build.VERSION.SDK_INT >= 30) {
            int light = WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS
                    | WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS;
            getWindow().getInsetsController().setSystemBarsAppearance(dark ? 0 : light, light);
        } else {
            getWindow().setStatusBarColor(bg);
            getWindow().setNavigationBarColor(bg);
            View d = getWindow().getDecorView();
            int flags = View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR | View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR;
            d.setSystemUiVisibility(dark ? d.getSystemUiVisibility() & ~flags : d.getSystemUiVisibility() | flags);
        }
    }

    /** Методы, доступные странице как window.ClepsyAndroid. */
    class Bridge {
        @JavascriptInterface
        public void openUrl(String url) {
            runOnUiThread(() -> openExternal(Uri.parse(url)));
        }

        @JavascriptInterface
        public void setDark(boolean dark, String bg) {
            int color;
            try {
                color = Color.parseColor(bg);
            } catch (Exception e) {
                color = dark ? 0xff0d121b : 0xfff2f5fa;
            }
            int c = color;
            runOnUiThread(() -> applyBars(dark, c));
        }
    }
}
