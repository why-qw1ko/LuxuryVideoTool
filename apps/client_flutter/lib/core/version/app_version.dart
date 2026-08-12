abstract final class AppVersion {
  static const current = String.fromEnvironment(
    'APP_VERSION',
    defaultValue: '0.1.0-dev',
  );

  static const commit = String.fromEnvironment(
    'APP_COMMIT',
    defaultValue: 'unknown',
  );
}

