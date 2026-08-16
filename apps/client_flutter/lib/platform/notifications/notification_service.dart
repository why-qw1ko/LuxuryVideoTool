import 'package:flutter_local_notifications/flutter_local_notifications.dart';

class NotificationService {
  final FlutterLocalNotificationsPlugin _plugin = FlutterLocalNotificationsPlugin();
  Future<void> initialise(void Function(String jobId) onJobSelected) async {
    const settings = InitializationSettings(
      android: AndroidInitializationSettings('ic_launcher'),
      windows: WindowsInitializationSettings(appName: 'Luxury Capture', appUserModelId: 'WhySoftware.DouyinCapture', guid: 'b16b55f2-25b2-4df6-b65d-7edfd4534190'),
    );
    await _plugin.initialize(settings: settings, onDidReceiveNotificationResponse: (response) { final payload = response.payload; if (payload != null) onJobSelected(payload); });
  }

  Future<void> showJob(CaptureJobNotification job) => _plugin.show(
    id: job.id.hashCode & 0x7fffffff,
    title: job.succeeded ? '任务已完成' : '任务失败',
    body: job.message,
    notificationDetails: const NotificationDetails(
      android: AndroidNotificationDetails('job_results', '任务结果', channelDescription: '任务完成和失败通知', importance: Importance.high, priority: Priority.high),
      windows: WindowsNotificationDetails(),
    ),
    payload: job.id,
  );
}

class CaptureJobNotification {
  const CaptureJobNotification({required this.id, required this.succeeded, required this.message});
  final String id;
  final bool succeeded;
  final String message;
}
