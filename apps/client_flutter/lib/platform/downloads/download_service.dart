import 'package:file_picker/file_picker.dart';
import 'package:path_provider/path_provider.dart';

class DownloadService {
  Future<String?> chooseDirectory() => FilePicker.platform.getDirectoryPath(dialogTitle: '选择下载目录');
  Future<String?> defaultDirectory() async => (await getDownloadsDirectory())?.path;
}
