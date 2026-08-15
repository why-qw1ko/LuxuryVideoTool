import 'package:douyin_capture/app/app_state.dart';
import 'package:douyin_capture/core/errors/app_failure.dart';
import 'package:douyin_capture/core/widgets/failure_view.dart';
import 'package:flutter/material.dart';

class LoginPage extends StatefulWidget {
  const LoginPage({required this.state, super.key});
  final AppState state;
  @override State<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends State<LoginPage> {
  final _username = TextEditingController();
  final _password = TextEditingController();
  AppFailure? _failure;
  bool _submitting = false;
  bool _obscurePassword = true;

  @override
  Widget build(BuildContext context) => Scaffold(
    body: Center(child: SingleChildScrollView(padding: const EdgeInsets.all(24), child: ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 420),
      child: AutofillGroup(child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: <Widget>[
        Icon(Icons.video_library_rounded, size: 72, color: Theme.of(context).colorScheme.primary),
        const SizedBox(height: 20), Text('Douyin Capture', textAlign: TextAlign.center, style: Theme.of(context).textTheme.headlineMedium),
        const SizedBox(height: 8), Text('登录后在 Android 与 Windows 同步任务和结果', textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyMedium),
        const SizedBox(height: 28),
        TextField(controller: _username, enabled: !_submitting, autofillHints: const <String>[AutofillHints.username], textInputAction: TextInputAction.next, decoration: const InputDecoration(labelText: '用户名', border: OutlineInputBorder())),
        const SizedBox(height: 12),
        TextField(controller: _password, enabled: !_submitting, obscureText: _obscurePassword, autofillHints: const <String>[AutofillHints.password], onSubmitted: (_) => _login(), decoration: InputDecoration(
          labelText: '密码',
          border: const OutlineInputBorder(),
          suffixIcon: IconButton(
            icon: Icon(_obscurePassword ? Icons.visibility_off : Icons.visibility),
            tooltip: _obscurePassword ? '显示密码' : '隐藏密码',
            onPressed: () => setState(() => _obscurePassword = !_obscurePassword),
          ),
        )),
        if (_failure != null) Padding(padding: const EdgeInsets.only(top: 12), child: FailureView(failure: _failure!)),
        const SizedBox(height: 18), FilledButton(onPressed: _submitting ? null : _login, child: _submitting ? const SizedBox.square(dimension: 20, child: CircularProgressIndicator(strokeWidth: 2)) : const Text('登录')),
        const SizedBox(height: 12), Text('服务器：${widget.state.settings.serverUrl}', textAlign: TextAlign.center),
      ])),
    ))),
  );

  Future<void> _login() async {
    if (_username.text.trim().isEmpty || _password.text.isEmpty) { setState(() => _failure = const AppFailure(title: '信息不完整', message: '请输入用户名和密码', nextStep: '补全后重试')); return; }
    setState(() { _submitting = true; _failure = null; });
    try { await widget.state.login(_username.text, _password.text); } on AppFailure catch (error) { if (mounted) setState(() => _failure = error); }
    if (mounted) setState(() => _submitting = false);
  }

  @override void dispose() { _username.dispose(); _password.dispose(); super.dispose(); }
}
