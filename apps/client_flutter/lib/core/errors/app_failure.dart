class AppFailure implements Exception {
  const AppFailure({
    required this.title,
    required this.message,
    required this.nextStep,
    this.code,
    this.requestId,
    this.retryable = false,
  });

  final String title;
  final String message;
  final String nextStep;
  final String? code;
  final String? requestId;
  final bool retryable;

  @override
  String toString() => message;
}
