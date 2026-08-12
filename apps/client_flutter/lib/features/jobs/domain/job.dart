class JobFile {
  const JobFile({required this.id, required this.kind, required this.name, required this.mimeType, required this.sizeBytes});
  factory JobFile.fromJson(Map<String, dynamic> json) => JobFile(
    id: json['id'] as String,
    kind: json['kind'] as String? ?? 'file',
    name: json['name'] as String? ?? 'download',
    mimeType: json['mimeType'] as String? ?? 'application/octet-stream',
    sizeBytes: (json['sizeBytes'] as num?)?.toInt() ?? 0,
  );
  final String id;
  final String kind;
  final String name;
  final String mimeType;
  final int sizeBytes;
}

class CaptureJob {
  const CaptureJob({
    required this.id,
    required this.action,
    required this.status,
    required this.progress,
    required this.statusMessage,
    required this.createdAt,
    required this.updatedAt,
    this.title = '',
    this.authorName = '',
    this.description = '',
    this.coverUrl = '',
    this.contentType = '',
    this.errorCode,
    this.errorMessage,
    this.result = const <String, dynamic>{},
    this.files = const <JobFile>[],
  });

  factory CaptureJob.fromJson(Map<String, dynamic> json) {
    final work = json['work'] as Map<String, dynamic>? ?? const <String, dynamic>{};
    final result = json['result'] as Map<String, dynamic>? ?? const <String, dynamic>{};
    final error = json['error'] as Map<String, dynamic>?;
    final rawFiles = (result['files'] as List<dynamic>?) ?? const <dynamic>[];
    return CaptureJob(
      id: json['id'] as String,
      action: json['action'] as String? ?? 'info',
      status: json['status'] as String? ?? 'queued',
      progress: (json['progress'] as num?)?.toInt() ?? 0,
      statusMessage: json['statusMessage'] as String? ?? '',
      createdAt: DateTime.tryParse(json['createdAt'] as String? ?? '') ?? DateTime.now(),
      updatedAt: DateTime.tryParse(json['updatedAt'] as String? ?? '') ?? DateTime.now(),
      title: work['title'] as String? ?? '',
      authorName: work['authorName'] as String? ?? '',
      description: work['description'] as String? ?? '',
      coverUrl: work['coverUrl'] as String? ?? '',
      contentType: work['type'] as String? ?? '',
      errorCode: error?['code'] as String?,
      errorMessage: error?['message'] as String?,
      result: result,
      files: rawFiles.map((item) => JobFile.fromJson(item as Map<String, dynamic>)).toList(),
    );
  }

  final String id;
  final String action;
  final String status;
  final int progress;
  final String statusMessage;
  final DateTime createdAt;
  final DateTime updatedAt;
  final String title;
  final String authorName;
  final String description;
  final String coverUrl;
  final String contentType;
  final String? errorCode;
  final String? errorMessage;
  final Map<String, dynamic> result;
  final List<JobFile> files;

  bool get terminal => status == 'completed' || status == 'failed' || status == 'cancelled';
  bool get canRetry => status == 'failed' || status == 'cancelled';
  bool get canCancel => !terminal;
  bool get hasResult => status == 'completed';
  String get displayTitle => title.isEmpty ? '任务 ${id.substring(0, id.length < 8 ? id.length : 8)}' : title;
}

class JobPage {
  const JobPage({required this.jobs, required this.total, required this.fromCache});
  final List<CaptureJob> jobs;
  final int total;
  final bool fromCache;
}
