import 'dart:async';

import 'package:douyin_capture/app/app_state.dart';
import 'package:douyin_capture/core/errors/app_failure.dart';
import 'package:douyin_capture/core/widgets/failure_view.dart';
import 'package:douyin_capture/features/jobs/domain/job.dart';
import 'package:douyin_capture/features/result/presentation/result_view.dart';
import 'package:douyin_capture/platform/notifications/notification_service.dart';
import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

class JobDetailPage extends StatefulWidget {
  const JobDetailPage({required this.state, required this.jobId, super.key});
  final AppState state; final String jobId;
  @override State<JobDetailPage> createState() => _JobDetailPageState();
}

class _JobDetailPageState extends State<JobDetailPage> with WidgetsBindingObserver {
  CaptureJob? _job; AppFailure? _failure; Timer? _timer; bool _foreground = true; String? _notifiedStatus;
  @override void initState() { super.initState(); WidgetsBinding.instance.addObserver(this); _load(); }
  @override void didChangeAppLifecycleState(AppLifecycleState state) { _foreground = state == AppLifecycleState.resumed; if (_job != null && !_job!.terminal) _schedule(); }
  Future<void> _load() async {
    _timer?.cancel();
    try {
      final job = await widget.state.jobs.get(widget.jobId);
      if (!mounted) return;
      setState(() { _job = job; _failure = null; });
      if (job.terminal && _notifiedStatus != null && _notifiedStatus != job.status) { if (widget.state.settings.notificationsEnabled) await widget.state.notifications.showJob(CaptureJobNotification(id: job.id, succeeded: job.status == 'completed', message: job.displayTitle)); }
      _notifiedStatus = job.status;
      if (!job.terminal) _schedule();
    } on AppFailure catch (error) { if (mounted) { setState(() => _failure = error); _timer = Timer(const Duration(seconds: 10), _load); } }
  }
  void _schedule() { _timer?.cancel(); final seconds = _foreground ? ((_job?.status == 'queued' || _job?.status == 'resolving') ? 2 : 3) : 20; _timer = Timer(Duration(seconds: seconds), _load); }

  @override Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('任务详情'), actions: <Widget>[IconButton(tooltip: '刷新', onPressed: _load, icon: const Icon(Icons.refresh)), if (_job?.terminal ?? false) IconButton(tooltip: '删除', onPressed: _delete, icon: const Icon(Icons.delete_outline))]),
    body: _job == null && _failure == null ? const Center(child: CircularProgressIndicator()) : ListView(padding: const EdgeInsets.all(20), children: <Widget>[
      if (_failure != null) FailureView(failure: _failure!, onRetry: _load),
      if (_job != null) ...<Widget>[
        Text(_job!.displayTitle, style: Theme.of(context).textTheme.headlineSmall), if (_job!.authorName.isNotEmpty) Text('@${_job!.authorName}'), const SizedBox(height: 16),
        Card(child: Padding(padding: const EdgeInsets.all(16), child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: <Widget>[
          Row(children: <Widget>[Icon(_icon(_job!.status)), const SizedBox(width: 8), Expanded(child: Text(_job!.statusMessage, style: Theme.of(context).textTheme.titleMedium)), Text('${_job!.progress}%')]),
          const SizedBox(height: 12), LinearProgressIndicator(value: _job!.progress / 100), const SizedBox(height: 12), _Timeline(status: _job!.status),
          if (_job!.errorMessage != null) Padding(padding: const EdgeInsets.only(top: 12), child: Text('${_job!.errorMessage}\n错误代码：${_job!.errorCode ?? '-'}', style: TextStyle(color: Theme.of(context).colorScheme.error))),
        ])),
        Wrap(spacing: 8, children: <Widget>[if (_job!.canCancel) OutlinedButton.icon(onPressed: _cancel, icon: const Icon(Icons.stop_circle_outlined), label: const Text('取消')), if (_job!.canRetry) FilledButton.tonalIcon(onPressed: _retry, icon: const Icon(Icons.replay), label: const Text('重试'))]),
        if (_job!.hasResult) ResultView(state: widget.state, job: _job!),
      ],
    ]),
  );
  IconData _icon(String status) => switch (status) { 'completed' => Icons.check_circle, 'failed' => Icons.error, 'cancelled' => Icons.cancel, _ => Icons.hourglass_top };
  Future<void> _cancel() async { try { await widget.state.jobs.cancel(widget.jobId); await _load(); } on AppFailure catch (e) { if (mounted) setState(() => _failure = e); } }
  Future<void> _retry() async { try { await widget.state.jobs.retry(widget.jobId); await _load(); } on AppFailure catch (e) { if (mounted) setState(() => _failure = e); } }
  Future<void> _delete() async { final confirmed = await showDialog<bool>(context: context, builder: (context) => AlertDialog(title: const Text('删除任务？'), content: const Text('任务记录和服务器上的相关文件会被删除，此操作无法撤销。'), actions: <Widget>[TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')), FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('删除'))])); if (confirmed != true) return; try { await widget.state.jobs.delete(widget.jobId); if (mounted) context.go('/history'); } on AppFailure catch (e) { if (mounted) setState(() => _failure = e); } }
  @override void dispose() { _timer?.cancel(); WidgetsBinding.instance.removeObserver(this); super.dispose(); }
}

class _Timeline extends StatelessWidget {
  const _Timeline({required this.status}); final String status;
  @override Widget build(BuildContext context) { const stages = <String>['queued', 'resolving', 'downloading', 'extracting', 'transcribing', 'postprocessing', 'completed']; final current = stages.indexOf(status); return Wrap(spacing: 6, runSpacing: 6, children: stages.map((stage) { final done = current >= stages.indexOf(stage) || status == 'completed'; return Chip(avatar: Icon(done ? Icons.check : Icons.circle_outlined, size: 16), label: Text(const <String, String>{'queued': '排队', 'resolving': '解析', 'downloading': '下载', 'extracting': '提取音频', 'transcribing': '转写', 'postprocessing': '生成结果', 'completed': '完成'}[stage]!)); }).toList()); }
}
