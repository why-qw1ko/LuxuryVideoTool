cmake_minimum_required(VERSION 3.14)

set(EPHEMERAL_DIR "${CMAKE_CURRENT_SOURCE_DIR}/ephemeral")
include(${EPHEMERAL_DIR}/generated_config.cmake)
set(WRAPPER_ROOT "${EPHEMERAL_DIR}/cpp_client_wrapper")

set(FLUTTER_LIBRARY "${EPHEMERAL_DIR}/flutter_windows.dll")
set(FLUTTER_LIBRARY_HEADERS
  "flutter_export.h"
  "flutter_windows.h"
  "flutter_messenger.h"
  "flutter_plugin_registrar.h"
  "flutter_texture_registrar.h"
)
list(TRANSFORM FLUTTER_LIBRARY_HEADERS PREPEND "${EPHEMERAL_DIR}/")
add_library(flutter INTERFACE)
target_include_directories(flutter INTERFACE "${EPHEMERAL_DIR}")
target_link_libraries(flutter INTERFACE "${FLUTTER_LIBRARY}.lib")
add_dependencies(flutter flutter_assemble)

list(APPEND FLUTTER_LIBRARY_HEADERS
  "${WRAPPER_ROOT}/core_implementations.cc"
  "${WRAPPER_ROOT}/standard_codec.cc"
)
list(APPEND CPP_WRAPPER_SOURCES_CORE
  "${WRAPPER_ROOT}/core_implementations.cc"
  "${WRAPPER_ROOT}/standard_codec.cc"
)
list(APPEND CPP_WRAPPER_SOURCES_PLUGIN
  "${WRAPPER_ROOT}/plugin_registrar.cc"
)
list(APPEND CPP_WRAPPER_SOURCES_APP
  "${WRAPPER_ROOT}/flutter_engine.cc"
  "${WRAPPER_ROOT}/flutter_view_controller.cc"
)

add_library(flutter_wrapper_plugin STATIC ${CPP_WRAPPER_SOURCES_CORE} ${CPP_WRAPPER_SOURCES_PLUGIN})
apply_standard_settings(flutter_wrapper_plugin)
set_target_properties(flutter_wrapper_plugin PROPERTIES POSITION_INDEPENDENT_CODE ON)
set_target_properties(flutter_wrapper_plugin PROPERTIES CXX_VISIBILITY_PRESET hidden)
target_link_libraries(flutter_wrapper_plugin PUBLIC flutter)
target_include_directories(flutter_wrapper_plugin PUBLIC "${WRAPPER_ROOT}/include")
add_dependencies(flutter_wrapper_plugin flutter_assemble)

add_library(flutter_wrapper_app STATIC ${CPP_WRAPPER_SOURCES_CORE} ${CPP_WRAPPER_SOURCES_APP})
apply_standard_settings(flutter_wrapper_app)
target_link_libraries(flutter_wrapper_app PUBLIC flutter)
target_include_directories(flutter_wrapper_app PUBLIC "${WRAPPER_ROOT}/include")
add_dependencies(flutter_wrapper_app flutter_assemble)

set(PHONY_OUTPUT "${CMAKE_CURRENT_BINARY_DIR}/_phony_")
set_source_files_properties("${PHONY_OUTPUT}" PROPERTIES SYMBOLIC TRUE)
add_custom_command(
  OUTPUT ${FLUTTER_LIBRARY} ${FLUTTER_LIBRARY_HEADERS} ${CPP_WRAPPER_SOURCES_CORE}
    ${CPP_WRAPPER_SOURCES_PLUGIN} ${CPP_WRAPPER_SOURCES_APP} ${PHONY_OUTPUT}
  COMMAND ${CMAKE_COMMAND} -E env ${FLUTTER_TOOL_ENVIRONMENT} "${FLUTTER_ROOT}/packages/flutter_tools/bin/tool_backend.bat" windows-x64 $<CONFIG>
  VERBATIM
)
add_custom_target(flutter_assemble DEPENDS "${FLUTTER_LIBRARY}" ${FLUTTER_LIBRARY_HEADERS}
  ${CPP_WRAPPER_SOURCES_CORE} ${CPP_WRAPPER_SOURCES_PLUGIN} ${CPP_WRAPPER_SOURCES_APP})

