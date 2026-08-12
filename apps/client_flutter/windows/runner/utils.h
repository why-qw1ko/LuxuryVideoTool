#ifndef RUNNER_UTILS_H_
#define RUNNER_UTILS_H_
#include <string>
#include <vector>
void CreateAndAttachConsole();
std::vector<std::string> GetCommandLineArguments();
int Scale(int source, double scale_factor);
#endif

