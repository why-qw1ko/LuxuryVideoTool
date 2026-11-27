package com.buleng.luxuryvideo.platform

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import com.buleng.luxuryvideo.MainActivity
import com.buleng.luxuryvideo.R

class CaptureNotifications(private val context: Context) {
    private val manager = context.getSystemService(NotificationManager::class.java)

    init {
        manager.createNotificationChannel(NotificationChannel(CHANNEL, "任务与下载", NotificationManager.IMPORTANCE_DEFAULT).apply {
            description = "内容处理结果和文件下载完成通知"
        })
    }

    fun jobCompleted(id: String, title: String, succeeded: Boolean) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) return
        if (!manager.areNotificationsEnabled()) return
        val intent = Intent(context, MainActivity::class.java).putExtra("job_id", id)
            .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP)
        val pending = PendingIntent.getActivity(context, id.hashCode(), intent, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        manager.notify(id.hashCode(), builder(if (succeeded) "任务处理完成" else "任务处理未完成", title)
            .setContentIntent(pending).build())
    }

    fun downloadCompleted(name: String, uri: Uri, mime: String) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) return
        if (!manager.areNotificationsEnabled()) return
        val intent = Intent(Intent.ACTION_VIEW).setDataAndType(uri, mime)
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
        val pending = PendingIntent.getActivity(context, uri.hashCode(), intent, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        manager.notify(uri.hashCode(), builder("下载完成", name).setContentIntent(pending).build())
    }

    private fun builder(title: String, text: String): Notification.Builder = Notification.Builder(context, CHANNEL)
        .setSmallIcon(R.drawable.ic_notification)
        .setContentTitle(title)
        .setContentText(text)
        .setAutoCancel(true)

    private companion object {
        const val CHANNEL = "capture_results"
    }
}
