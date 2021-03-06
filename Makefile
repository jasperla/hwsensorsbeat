BEATNAME=hwsensorsbeat
BEAT_DIR=github.com/jasperla
SYSTEM_TESTS=false
TEST_ENVIRONMENT=false
ES_BEATS=./vendor/github.com/elastic/beats
GOPACKAGES=$(shell glide novendor)
PREFIX?=.

# Path to the libbeat Makefile
-include $(ES_BEATS)/libbeat/scripts/Makefile

.PHONY: update-deps
update-deps:
	glide update  --no-recursive

# This is called by the beats packer before building starts
.PHONY: before-build
before-build:
