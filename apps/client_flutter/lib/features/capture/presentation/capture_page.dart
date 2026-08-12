import 'package:douyin_capture/app/app_state.dart';
import 'package:douyin_capture/core/errors/app_failure.dart';
import 'package:douyin_capture/core/widgets/failure_view.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:go_router/go_router.dart';

class CapturePage extends StatefulWidget {
  const CapturePage({required this.state, super.key});
  final AppState state;
  @override State<CapturePage> createState() => _CapturePageState();
}

class _CapturePageState extends State<CapturePage> {
  final _text = TextEditingController();
  String _action = 'full';
  bool _keepVideo = false;
  bool _submitting = false;
  AppFailure? _failure;

  @override
  void initState() { super.initState(); WidgetsBinding.instance.addPostFrameCallback((_) => _consumeShare()); widget.state.addListener(_stateChanged); }
  void _stateChanged() { if (widget.state.pendingShareText != null) _consumeShare(); }
  void _consumeShare() { final shared = widget.state.takePendingShare(); if (shared != null && mounted) { _text.text = shared; setState(() {}); } }

  @override
  Widget build(BuildContext context) => ListView(padding: const EdgeInsets.all(20), children: <Widget>[
    Text('提取抖音内容', style: Theme.of(context).textTheme.headlineSmall),
    const SizedBox(height: 6), const Text('粘贴分享文本，或在 Android 抖音分享面板选择本应用。提交前由你确认，不会自动产生转写费用。'),
    const SizedBox(height: 20),
    TextField(controller: _text, minLines: 4, maxLines: 8, enabled: !_submitting, decoration: InputDecoration(labelText: '抖音分享文本', hintText: '复制打开抖音 https://v.douyin.com/...', border: const OutlineInputBorder(), suffixIcon: IconButton(tooltip: '从剪贴板粘贴', onPressed: _paste, icon: const Icon(Icons.content_paste)))),
    const SizedBox(height: 16), SegmentedButton<String>(segments: const <ButtonSegment<String>>[
      ButtonSegment(value: 'info', label: Text('仅解析'), icon: Icon(Icons.info_outline)),
      ButtonSegment(value: 'download', label: Text('下载'), icon: Icon(Icons.download_outlined)),
      ButtonSegment(value: 'transcribe', label: Text('转写'), icon: Icon(Icons.graphic_eq)),
      ButtonSegment(value: 'full', label: Text('完整流程'), icon: Icon(Icons.auto_awesome)),
    ], selected: <String>{_action}, onSelectionChanged: _submitting ? null : (value) => setState(() => _action = value.first)),
    if (_action == 'full' || _action == 'transcribe') SwitchListTile(contentPadding: EdgeInsets.zero, title: const Text('同时保留视频'), subtitle: const Text('默认仅保留转写结果，以减少服务器空间占用'), value: _keepVideo, onChanged: _submitting ? null : (value) => setState(() => _keepVideo = value)),
    if (_failure != null) FailureView(failure: _failure!, onRetry: _submit),
    const SizedBox(height: 16), FilledButton.icon(onPressed: _submitting ? null : _submit, icon: _submitting ? const SizedBox.square(dimension: 18, child: CircularProgressIndicator(strokeWidth: 2)) : const Icon(Icons.send), label: const Text('创建任务')),
  ]);

  Future<void> _paste() async { final value = await Clipboard.getData(Clipboard.kTextPlain); if (value?.text != null) _text.text = value!.text!; }
  Future<void> _submit() async {
    if (_text.text.trim().isEmpty) { setState(() => _failure = const AppFailure(title: '还没有链接', message: '未填写抖音分享文本', nextStep: '粘贴完整分享文本后再提交')); return; }
    setState(() { _submitting = true; _failure = null; });
    try { final job = await widget.state.jobs.create(shareText: _text.text.trim(), action: _action, keepVideo: _keepVideo); if (mounted) context.go('/jobs/${job.id}'); } on AppFailure catch (error) { if (mounted) setState(() => _failure = error); }
    if (mounted) setState(() => _submitting = false);
  }
  @override void dispose() { widget.state.removeListener(_stateChanged); _text.dispose(); super.dispose(); }
}
