#  Copyright 2023 Google Inc. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.

top_srcdir = ./
MAKEFLAGS += -r --no-print-directory

SKIP_LIB_TARGETS ?= "false"

# User may want to re-define, if they don't then assume default values.
PREFIX ?= $(top_srcdir)out/
USRDIR ?= $(PREFIX)usr/
BINDIR ?= $(USRDIR)bin/
ETCDIR ?= $(PREFIX)etc/
INITDIR ?= $(PREFIX)etc/init/
UNITDIR ?= $(USRDIR)lib/systemd/system/
PRESETDIR ?= $(USRDIR)lib/systemd/system-preset/

# Go commands definition variables - we don't advertise in the help
# but the user can change it.
GO_CMD ?= go
GO_BUILD_CMD ?= $(GO_CMD) build
GO_CLEAN_CMD ?= $(GO_CMD) clean
GO_TEST_CMD ?= $(GO_CMD) test

# Defines the command execution verbosity, if VERBOSE is defined
# then the command execution will not be suppressed.
ifndef VERBOSE
Q = @
else
GO_BUILD_CMD += -x
GO_CLEAN_CMD += -x
GO_TEST_CMD += -x
endif

parse-command-test = \
	$(eval $(1)-exit-code := $(shell $(2) &> /dev/null; echo $$?)) \
	$(eval all-commands += $(1)) \

make-command-test = \
	$(eval $(call parse-command-test,golang,$(GO_CMD) version)) \

$(call make-command-test)

$(foreach command,$(all-commands), \
	$(if $(filter-out "0","$($(command)-exit-code)"), \
		$(eval $(error "command not found: $(command)")) \
	) \
) \

include $(top_srcdir)build/root/Makefile.rules

.DEFAULT_GOAL = all
