import 'package:flutter/material.dart';

class BootstrapPage extends StatelessWidget {
  const BootstrapPage({required this.version, super.key});

  final String version;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Luxury Capture')),
      body: Center(
        child: Semantics(
          label: '应用工程基线已就绪',
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.video_library_outlined, size: 64),
              const SizedBox(height: 16),
              Text('工程基线已就绪', style: Theme.of(context).textTheme.headlineSmall),
              const SizedBox(height: 8),
              Text('版本 $version'),
            ],
          ),
        ),
      ),
    );
  }
}

