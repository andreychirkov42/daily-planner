package io.github.andreychirkov42.clepsy;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageInstaller;
import android.os.Build;

/**
 * Ответ системного установщика на обновление. Приёмник закрыт (exported=false): дойти до него может
 * только наш PendingIntent. Если принимать ответ в MainActivity (она открыта для всех), чужое
 * приложение подсунуло бы свой EXTRA_INTENT и заставило Clepsy открыть что угодно от своего имени.
 */
public class InstallReceiver extends BroadcastReceiver {
    @Override
    @SuppressWarnings("deprecation")
    public void onReceive(Context ctx, Intent intent) {
        int st = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE);
        if (st == PackageInstaller.STATUS_PENDING_USER_ACTION) {
            Intent confirm = Build.VERSION.SDK_INT >= 33
                    ? intent.getParcelableExtra(Intent.EXTRA_INTENT, Intent.class)
                    : intent.getParcelableExtra(Intent.EXTRA_INTENT);
            if (confirm == null) return;
            confirm.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            ctx.startActivity(confirm); // окно «Обновить приложение?»
        } else if (st != PackageInstaller.STATUS_SUCCESS) {
            MainActivity.installFailed(st == PackageInstaller.STATUS_FAILURE_ABORTED ? ""
                    : "установка не удалась: " + intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE));
        }
    }
}
