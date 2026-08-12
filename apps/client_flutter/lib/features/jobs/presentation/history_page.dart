import 'package:douyin_capture/app/app_state.dart';
import 'package:douyin_capture/core/errors/app_failure.dart';
import 'package:douyin_capture/core/widgets/failure_view.dart';
import 'package:douyin_capture/features/jobs/domain/job.dart';
import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

class HistoryPage extends StatefulWidget {
  const HistoryPage({required this.state, super.key});
  final AppState state;
  @override State<HistoryPage> createState() => _HistoryPageState();
}

class _HistoryPageState extends State<HistoryPage> {
  final _query = TextEditingController();
  String _status = '';
  String _action = '';
  JobPage? _page;
  AppFailure? _failure;
  bool _loading = true;

  @override void initState() { super.initState(); _load(); }
  Future<void> _load() async {
    setState(() { _loading = true; _failure = null; });
    try { final page = await widget.state.jobs.list(query: _query.text, status: _status, action: _action); if (mounted) setState(() => _page = page); } on AppFailure catch (error) { if (mounted) setState(() => _failure = error); }
    if (mounted) setState(() => _loading = false);
  }

  @override Widget build(BuildContext context) => RefreshIndicator(onRefresh: _load, child: ListView(padding: const EdgeInsets.all(16), children: <Widget>[
    Row(children: <Widget>[Expanded(child: TextField(controller: _query, onSubmitted: (_) => _load(), decoration: const InputDecoration(prefixIcon: Icon(Icons.search), hintText: '搜索标题或作者', border: OutlineInputBorder()))), const SizedBox(width: 8), IconButton.filledTonal(tooltip: '刷新', onPressed: _load, icon: const Icon(Icons.refresh))]),
    const SizedBox(height: 12), Wrap(spacing: 8, runSpacing: 8, children: <Widget>[
      DropdownMenu<String>(initialSelection: _status, label: const Text('状态'), dropdownMenuEntries: const <DropdownMenuEntry<String>>[DropdownMenuEntry(value: '', label: '全部'), DropdownMenuEntry(value: 'completed', label: '已完成'), DropdownMenuEntry(value: 'failed', label: '失败'), DropdownMenuEntry(value: 'queued', label: '排队中')], onSelected: (value) { _status = value ?? ''; _load(); }),
      DropdownMenu<String>(initialSelection: _action, label: const Text('类型'), dropdownMenuEntries: const <DropdownMenuEntry<String>>[DropdownMenuEntry(value: '', label: '全部'), DropdownMenuEntry(value: 'info', label: '解析'), DropdownMenuEntry(value: 'download', label: '下载'), DropdownMenuEntry(value: 'transcribe', label: '转写'), DropdownMenuEntry(value: 'full', label: '完整流程')], onSelected: (value) { _action = value ?? ''; _load(); }),
    ]),
    if (_page?.fromCache ?? false) const Card(child: ListTile(leading: Icon(Icons.cloud_off), title: Text('当前为离线历史'), subtitle: Text('网络恢复后下拉刷新即可同步'))),
    if (_loading) const Padding(padding: EdgeInsets.all(32), child: Center(child: CircularProgressIndicator())),
    if (_failure != null) FailureView(failure: _failure!, onRetry: _load),
    if (!_loading && _failure == null && (_page?.jobs.isEmpty ?? true)) const Padding(padding: EdgeInsets.all(40), child: Center(child: Text('暂无匹配的历史任务'))),
    ...?_page?.jobs.map((job) => Card(child: ListTile(onTap: () => context.go('/jobs/${job.id}'), leading: _statusIcon(job.status), title: Text(job.displayTitle, maxLines: 1, overflow: TextOverflow.ellipsis), subtitle: Text('${_actionName(job.action)} · ${job.statusMessage}\n${_formatTime(job.createdAt)}'), isThreeLine: true, trailing: const Icon(Icons.chevron_right)))),
  ]));

  Widget _statusIcon(String status) => Icon(switch (status) { 'completed' => Icons.check_circle, 'failed' => Icons.error, 'cancelled' => Icons.cancel, _ => Icons.pending }, color: switch (status) { 'completed' => Colors.green, 'failed' => Colors.red, _ => null });
  String _actionName(String action) => const <String, String>{'info': '解析', 'download': '下载', 'transcribe': '转写', 'full': '完整流程'}[action] ?? action;
  String _formatTime(DateTime value) => '${value.toLocal().year}-${value.toLocal().month.toString().padLeft(2, '0')}-${value.toLocal().day.toString().padLeft(2, '0')} ${value.toLocal().hour.toString().padLeft(2, '0')}:${value.toLocal().minute.toString().padLeft(2, '0')}';
  @override void dispose() { _query.dispose(); super.dispose(); }
}
