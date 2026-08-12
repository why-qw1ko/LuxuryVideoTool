import 'package:douyin_capture/app/app_state.dart';
import 'package:douyin_capture/core/errors/app_failure.dart';
import 'package:douyin_capture/features/jobs/domain/job.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class ResultView extends StatefulWidget {
  const ResultView({required this.state, required this.job, super.key});
  final AppState state;
  final CaptureJob job;
  @override State<ResultView> createState() => _ResultViewState();
}

class _ResultViewState extends State<ResultView> {
  double? _downloadProgress;
  @override Widget build(BuildContext context) {
    final transcript = widget.job.result['normalizedText'] as String? ?? widget.job.result['transcript'] as String? ?? widget.job.result['text'] as String? ?? '';
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: <Widget>[
      if (widget.job.description.isNotEmpty) _TextResult(title: '发布文案', text: widget.job.description),
      if (transcript.isNotEmpty) _TextResult(title: '口播文案', text: transcript),
      if (widget.job.files.isNotEmpty) ...<Widget>[
        const SizedBox(height: 16), Text('结果文件', style: Theme.of(context).textTheme.titleMedium),
        if (_downloadProgress != null) LinearProgressIndicator(value: _downloadProgress),
        ...widget.job.files.map((file) => ListTile(contentPadding: EdgeInsets.zero, leading: const Icon(Icons.insert_drive_file_outlined), title: Text(file.name), subtitle: Text('${file.kind} · ${_size(file.sizeBytes)}'), trailing: IconButton(tooltip: '下载', onPressed: () => _download(file), icon: const Icon(Icons.download)))),
      ],
    ]);
  }
  Future<void> _download(JobFile file) async {
    var directory = widget.state.settings.downloadDirectory;
    directory ??= await widget.state.downloads.defaultDirectory();
    directory ??= await widget.state.downloads.chooseDirectory();
    if (directory == null) return;
    try {
      final path = await widget.state.jobs.download(file, directory, (received, total) { if (mounted && total > 0) setState(() => _downloadProgress = received / total); });
      if (mounted) { setState(() => _downloadProgress = null); ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('已保存到 $path'))); }
    } on AppFailure catch (failure) { if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('${failure.title}：${failure.message}'))); }
  }
  String _size(int bytes) => bytes < 1024 * 1024 ? '${(bytes / 1024).toStringAsFixed(1)} KB' : '${(bytes / 1024 / 1024).toStringAsFixed(1)} MB';
}

class _TextResult extends StatelessWidget {
  const _TextResult({required this.title, required this.text});
  final String title; final String text;
  @override Widget build(BuildContext context) => Card(child: Padding(padding: const EdgeInsets.all(16), child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: <Widget>[Row(children: <Widget>[Expanded(child: Text(title, style: Theme.of(context).textTheme.titleMedium)), IconButton(tooltip: '复制', onPressed: () { Clipboard.setData(ClipboardData(text: text)); ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('已复制'))); }, icon: const Icon(Icons.copy))]), SelectableText(text)])));
}
