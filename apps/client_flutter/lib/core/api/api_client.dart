import 'dart:async';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:douyin_capture/core/auth/token_store.dart';
import 'package:douyin_capture/core/errors/app_failure.dart';
import 'package:douyin_capture/core/version/app_version.dart';

typedef SessionChanged = Future<void> Function(AuthSession? session);

class ApiClient {
  ApiClient({required String baseUrl, required TokenStore tokenStore, required SessionChanged onSessionChanged})
    : _tokenStore = tokenStore,
      _onSessionChanged = onSessionChanged,
      _dio = Dio(BaseOptions(
        baseUrl: _normaliseBaseUrl(baseUrl),
        connectTimeout: const Duration(seconds: 10),
        receiveTimeout: const Duration(seconds: 30),
        sendTimeout: const Duration(seconds: 15),
        headers: <String, dynamic>{
          'X-App-Version': AppVersion.current,
          'X-Platform': Platform.isAndroid ? 'android' : 'windows',
          'User-Agent': 'DouyinCapture/${AppVersion.current}',
        },
      ));

  final Dio _dio;
  final TokenStore _tokenStore;
  final SessionChanged _onSessionChanged;
  Completer<AuthSession?>? _refreshing;

  static String _normaliseBaseUrl(String value) => value.trim().replaceAll(RegExp(r'/+$'), '');

  Future<Map<String, dynamic>> request(
    String method,
    String path, {
    Map<String, dynamic>? data,
    Map<String, dynamic>? query,
    Map<String, dynamic>? headers,
    bool authenticated = true,
    bool allowRefresh = true,
  }) async {
    try {
      final requestHeaders = <String, dynamic>{...?headers};
      if (authenticated) {
        final session = await _tokenStore.read();
        if (session == null) throw const AppFailure(title: '需要登录', message: '登录状态已失效', nextStep: '请重新登录');
        requestHeaders['Authorization'] = 'Bearer ${session.accessToken}';
      }
      final response = await _dio.request<Map<String, dynamic>>(
        path,
        data: data,
        queryParameters: query,
        options: Options(method: method, headers: requestHeaders),
      );
      return response.data ?? <String, dynamic>{};
    } on DioException catch (error) {
      if (authenticated && allowRefresh && error.response?.statusCode == 401) {
        final session = await _refresh();
        if (session != null) return request(method, path, data: data, query: query, headers: headers, allowRefresh: false);
      }
      throw _failure(error);
    }
  }

  Future<AuthSession?> _refresh() async {
    if (_refreshing != null) return _refreshing!.future;
    final completer = Completer<AuthSession?>();
    _refreshing = completer;
    try {
      final current = await _tokenStore.read();
      if (current == null || current.refreshExpiresAt.isBefore(DateTime.now())) { await _expireSession(); completer.complete(null); return null; }
      final data = await request('POST', '/api/v1/auth/refresh', data: <String, dynamic>{'refreshToken': current.refreshToken}, authenticated: false, allowRefresh: false);
      final session = AuthSession.fromJson(data);
      await _tokenStore.write(session);
      await _onSessionChanged(session);
      completer.complete(session);
      return session;
    } catch (_) {
      await _expireSession();
      completer.complete(null);
      return null;
    } finally {
      _refreshing = null;
    }
  }

  Future<void> _expireSession() async { await _tokenStore.clear(); await _onSessionChanged(null); }

  Future<void> download(String fileId, String savePath, void Function(int received, int total) progress, {bool allowRefresh = true}) async {
    final session = await _tokenStore.read();
    if (session == null) throw const AppFailure(title: '需要登录', message: '登录状态已失效', nextStep: '请重新登录');
    try {
      await _dio.download('/api/v1/files/$fileId', savePath, options: Options(headers: <String, dynamic>{'Authorization': 'Bearer ${session.accessToken}'}), onReceiveProgress: progress);
    } on DioException catch (error) {
      if (allowRefresh && error.response?.statusCode == 401 && await _refresh() != null) { return download(fileId, savePath, progress, allowRefresh: false); }
      throw _failure(error);
    }
  }

  AppFailure _failure(DioException error) {
    final body = error.response?.data;
    final envelope = body is Map<String, dynamic> ? body : <String, dynamic>{};
    final nested = envelope['error'];
    final json = nested is Map<String, dynamic> ? nested : envelope;
    final code = json['code'] as String?;
    final requestId = json['requestId'] as String? ?? error.response?.headers.value('x-request-id');
    if (error.type == DioExceptionType.connectionError || error.type == DioExceptionType.connectionTimeout || error.type == DioExceptionType.receiveTimeout) {
      return AppFailure(title: '无法连接服务器', message: '网络不可用或服务器暂时无响应', nextStep: '检查网络后重试；历史记录仍可离线查看', code: code, requestId: requestId, retryable: true);
    }
    return AppFailure(
      title: _titleFor(code),
      message: json['message'] as String? ?? '请求未能完成',
      nextStep: _nextStepFor(code),
      code: code,
      requestId: requestId,
      retryable: json['retryable'] as bool? ?? false,
    );
  }

  String _titleFor(String? code) => switch (code) {
    'INVALID_SHARE_LINK' => '没有找到有效链接',
    'AUTH_INVALID_CREDENTIALS' => '登录失败',
    'ASR_BUDGET_EXCEEDED' => '转写额度不足',
    'CLIENT_UPGRADE_REQUIRED' => '需要更新客户端',
    _ => '操作失败',
  };
  String _nextStepFor(String? code) => switch (code) {
    'INVALID_SHARE_LINK' => '重新复制完整的抖音分享文本',
    'AUTH_INVALID_CREDENTIALS' => '检查用户名和密码后重试',
    'ASR_BUDGET_EXCEEDED' => '可先仅解析或下载，或联系管理员增加额度',
    'CLIENT_UPGRADE_REQUIRED' => '安装管理员提供的最新版本',
    _ => '稍后重试；持续失败时复制请求 ID 给管理员',
  };
}
