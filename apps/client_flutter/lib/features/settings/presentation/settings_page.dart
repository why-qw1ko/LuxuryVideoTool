import 'package:douyin_capture/app/app_state.dart';
import 'package:douyin_capture/core/storage/settings_store.dart';
import 'package:flutter/material.dart';

class SettingsPage extends StatefulWidget {
  const SettingsPage({required this.state, super.key}); final AppState state;
  @override State<SettingsPage> createState() => _SettingsPageState();
}
class _SettingsPageState extends State<SettingsPage> {
  late final TextEditingController _server;
  @override void initState() { super.initState(); _server = TextEditingController(text: widget.state.settings.serverUrl); }
  @override Widget build(BuildContext context) { final settings = widget.state.settings; return ListView(padding: const EdgeInsets.all(20), children: <Widget>[
    Text('设置', style: Theme.of(context).textTheme.headlineSmall), const SizedBox(height: 16),
    TextField(controller: _server, keyboardType: TextInputType.url, decoration: const InputDecoration(labelText: '服务器地址', helperText: '生产环境必须使用 HTTPS', border: OutlineInputBorder()), onSubmitted: (_) => _save(serverUrl: _server.text)),
    const SizedBox(height: 12), ListTile(contentPadding: EdgeInsets.zero, leading: const Icon(Icons.folder_outlined), title: const Text('下载目录'), subtitle: Text(settings.downloadDirectory ?? '使用系统下载目录'), trailing: TextButton(onPressed: _chooseDirectory, child: const Text('选择'))),
    SwitchListTile(contentPadding: EdgeInsets.zero, secondary: const Icon(Icons.notifications_outlined), title: const Text('任务完成通知'), value: settings.notificationsEnabled, onChanged: (value) => _save(notificationsEnabled: value)),
    ListTile(contentPadding: EdgeInsets.zero, leading: const Icon(Icons.palette_outlined), title: const Text('主题'), trailing: DropdownButton<AppThemeMode>(value: settings.themeMode, items: const <DropdownMenuItem<AppThemeMode>>[DropdownMenuItem(value: AppThemeMode.system, child: Text('跟随系统')), DropdownMenuItem(value: AppThemeMode.light, child: Text('浅色')), DropdownMenuItem(value: AppThemeMode.dark, child: Text('深色'))], onChanged: (value) { if (value != null) _save(themeMode: value); })),
    const Divider(), ListTile(contentPadding: EdgeInsets.zero, leading: const Icon(Icons.person_outline), title: Text(widget.state.session?.displayName ?? ''), subtitle: Text(widget.state.session?.role ?? '')), OutlinedButton.icon(onPressed: widget.state.logout, icon: const Icon(Icons.logout), label: const Text('退出登录')),
  ]); }
  Future<void> _chooseDirectory() async { final value = await widget.state.downloads.chooseDirectory(); if (value != null) await _save(downloadDirectory: value); }
  Future<void> _save({String? serverUrl, String? downloadDirectory, bool? notificationsEnabled, AppThemeMode? themeMode}) => widget.state.updateSettings(widget.state.settings.copyWith(serverUrl: serverUrl, downloadDirectory: downloadDirectory, notificationsEnabled: notificationsEnabled, themeMode: themeMode));
  @override void dispose() { _server.dispose(); super.dispose(); }
}
