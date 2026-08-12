import 'package:douyin_capture/features/jobs/domain/job.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('parses a completed transcription result', () {
    final job = CaptureJob.fromJson(<String, dynamic>{
      'id': 'job-12345678',
      'action': 'full',
      'status': 'completed',
      'progress': 100,
      'statusMessage': '完成',
      'createdAt': '2026-08-12T00:00:00Z',
      'updatedAt': '2026-08-12T00:01:00Z',
      'work': <String, dynamic>{'title': '标题', 'authorName': '作者'},
      'result': <String, dynamic>{'normalizedText': '转写内容', 'files': <dynamic>[<String, dynamic>{'id': 'file-1', 'kind': 'markdown', 'name': 'result.md', 'mimeType': 'text/markdown', 'sizeBytes': 12}]},
    });
    expect(job.terminal, isTrue);
    expect(job.displayTitle, '标题');
    expect(job.files.single.name, 'result.md');
  });
}
