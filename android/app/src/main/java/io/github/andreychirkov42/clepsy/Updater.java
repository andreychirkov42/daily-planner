package io.github.andreychirkov42.clepsy;

import android.content.Context;

import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.security.MessageDigest;

/**
 * Обновление приложения из релизов GitHub: прочитать манифест latest.json последнего релиза
 * (по постоянной ссылке, без API GitHub), скачать сборку "android" и сверить её SHA-256 с манифестом.
 * Ставит APK системный установщик (MainActivity); подмену отсекает сама система — обновление
 * принимается только с той же подписью APK. Проверка — здесь, а не в Go-сервере: у него на телефоне нет DNS.
 */
final class Updater {
    static final String MANIFEST = "https://github.com/andreychirkov42/daily-planner/releases/latest/download/latest.json";
    static final long MAX_APK = 200L << 20;

    final Context ctx;
    final String current;
    String latest = "", notes = "", state = "", error = "";
    String updatedFrom = ""; // с какой версии обновились (страница покажет «Что нового»)
    boolean available;
    long checked;

    Updater(Context ctx, String current) {
        this.ctx = ctx;
        this.current = current;
    }

    File apk() { return new File(ctx.getCacheDir(), "update.apk"); }

    /** Состояние меняют несколько потоков (проверка, установщик, страница) — только под замком. */
    synchronized void setError(String newState, String err) {
        state = newState;
        error = err;
    }

    synchronized JSONObject json() {
        JSONObject o = new JSONObject();
        try {
            o.put("current", current).put("latest", latest).put("notes", notes).put("available", available)
                    .put("state", state).put("error", error).put("canInstall", true);
            if (!updatedFrom.isEmpty()) o.put("updatedFrom", updatedFrom);
            if (checked > 0) o.put("checked", new java.text.SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ssXXX", java.util.Locale.US).format(new java.util.Date(checked)));
        } catch (Exception ignored) {
        }
        return o;
    }

    /** Узнаёт последнюю версию и, если она новее, скачивает и проверяет APK. Долго — не из главного потока. */
    void check() {
        synchronized (this) {
            if (state.equals("checking") || state.equals("downloading")) return;
            state = "checking";
            error = "";
        }
        try {
            JSONObject m = new JSONObject(new String(get(MANIFEST, 1 << 20), "UTF-8"));
            String version = m.getString("version").replaceFirst("^v", "");
            JSONObject build = m.optJSONObject("platforms") == null ? null : m.getJSONObject("platforms").optJSONObject("android");
            boolean avail = newer(version, current);
            synchronized (this) {
                latest = version;
                notes = m.optString("notes", "");
                available = avail;
                checked = System.currentTimeMillis();
                state = avail ? "downloading" : "";
            }
            if (!avail) return;
            if (build == null) throw new IOException("в версии " + version + " нет сборки для Android");
            byte[] b = get(build.getString("url"), MAX_APK);
            if (!build.getString("sha256").toLowerCase(java.util.Locale.ROOT).equals(sha256(b)))
                throw new IOException("контрольная сумма не совпала — файл повреждён или подменён, не устанавливаю");
            try (FileOutputStream f = new FileOutputStream(apk())) {
                f.write(b);
            }
            synchronized (this) {
                state = "ready";
            }
        } catch (Exception e) {
            synchronized (this) {
                state = "error";
                error = e instanceof IOException && e.getMessage() != null ? e.getMessage() : "нет связи с GitHub";
                checked = System.currentTimeMillis();
            }
        }
    }

    static byte[] get(String url, long limit) throws IOException {
        HttpURLConnection c = (HttpURLConnection) new URL(url).openConnection();
        c.setConnectTimeout(15000);
        c.setReadTimeout(60000);
        c.setRequestProperty("User-Agent", "Clepsy-Android");
        try {
            if (c.getResponseCode() == 404) throw new IOException("в последнем релизе нет latest.json — обнови вручную со страницы релизов");
            if (c.getResponseCode() != 200) throw new IOException("GitHub ответил " + c.getResponseCode());
            try (InputStream in = c.getInputStream(); ByteArrayOutputStream out = new ByteArrayOutputStream()) {
                byte[] buf = new byte[64 << 10];
                long total = 0;
                for (int n; (n = in.read(buf)) > 0; ) {
                    total += n;
                    if (total > limit) throw new IOException("файл слишком большой");
                    out.write(buf, 0, n);
                }
                return out.toByteArray();
            }
        } catch (java.net.UnknownHostException e) {
            throw new IOException("нет интернета");
        } finally {
            c.disconnect();
        }
    }

    static String sha256(byte[] b) throws Exception {
        StringBuilder sb = new StringBuilder();
        for (byte x : MessageDigest.getInstance("SHA-256").digest(b)) sb.append(String.format("%02x", x));
        return sb.toString();
    }

    /** Новее ли a, чем b; та же логика, что newerVersion в update.go. */
    static boolean newer(String a, String b) {
        int[] na = new int[3], nb = new int[3];
        String pa = parse(a, na), pb = parse(b, nb);
        if (pa == null || pb == null) return false;
        for (int i = 0; i < 3; i++) if (na[i] != nb[i]) return na[i] > nb[i];
        if (pa.equals(pb)) return false;
        if (pa.isEmpty()) return true;
        if (pb.isEmpty()) return false;
        return pa.compareTo(pb) > 0;
    }

    private static String parse(String v, int[] nums) {
        v = v.trim().replaceFirst("^v", "");
        String pre = "";
        int dash = v.indexOf('-');
        if (dash >= 0) {
            pre = v.substring(dash + 1);
            v = v.substring(0, dash);
        }
        String[] p = v.split("\\.");
        if (p.length != 3) return null;
        try {
            for (int i = 0; i < 3; i++) nums[i] = Integer.parseInt(p[i]);
        } catch (NumberFormatException e) {
            return null;
        }
        return pre;
    }
}
