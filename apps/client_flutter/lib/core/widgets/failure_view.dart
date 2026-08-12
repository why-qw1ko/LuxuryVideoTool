import 'package:douyin_capture/core/errors/app_failure.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class FailureView extends StatelessWidget {
  const FailureView({required this.failure, this.onRetry, super.key});
  final AppFailure failure;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) => Card(
    color: Theme.of(context).colorScheme.errorContainer,
    child: Padding(
      padding: const EdgeInsets.all(16),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: <Widget>[
        Text(failure.title, style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 6), Text(failure.message),
        const SizedBox(height: 6), Text('下一步：${failure.nextStep}'),
        if (failure.requestId != null) TextButton.icon(onPressed: () => Clipboard.setData(ClipboardData(text: failure.requestId!)), icon: const Icon(Icons.copy), label: Text('复制请求 ID：${failure.requestId}')),
        if (onRetry != null) FilledButton.tonal(onPressed: onRetry, child: const Text('重试')),
      ]),
    ),
  );
}
