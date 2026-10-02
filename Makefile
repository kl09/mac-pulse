.PHONY: build run test lint bundle app universal install dmg zip checksums sign notarize release clean

APP := dist/mac-pulse.app
# One source for the version: the update check compares it with a release tag.
VERSION := $(shell plutil -extract CFBundleShortVersionString raw packaging/Info.plist)
LDFLAGS := -X main.version=$(VERSION)

# Must match LSMinimumSystemVersion in packaging/Info.plist: clang then rejects newer APIs used without @available.
# Flags rather than MACOSX_DEPLOYMENT_TARGET, which go's build cache does not see.
export CGO_CFLAGS := -g -O2 -mmacosx-version-min=13.0
export CGO_LDFLAGS := -g -O2 -mmacosx-version-min=13.0

# Release artefacts are named after the architecture inside them: arm64 on Apple Silicon,
# "universal" after `make universal dmg zip checksums ARCH=universal`.
ARCH ?= $(shell uname -m)
DMG := dist/mac-pulse-$(VERSION)-$(ARCH).dmg
ZIP := dist/mac-pulse-$(VERSION)-$(ARCH).zip

# Optional, both empty by default (see docs/RELEASING.md):
#   DEVELOPER_ID    a signing identity such as "Developer ID Application: Name (TEAMID)"
#   NOTARY_PROFILE  a keychain profile made with `xcrun notarytool store-credentials`
DEVELOPER_ID ?=
NOTARY_PROFILE ?=

# Without DEVELOPER_ID the signature is ad hoc: enough to run locally, and it changes with every build.
# The hardened runtime keeps another program of the same user from loading its code into mac-pulse
# through DYLD_INSERT_LIBRARIES. A Developer ID signature also needs a secure timestamp to be notarized.
SIGN := codesign --force --options runtime --entitlements packaging/entitlements.plist \
	$(if $(DEVELOPER_ID),--timestamp -s "$(DEVELOPER_ID)",-s -)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/mac-pulse ./cmd/mac-pulse
	$(SIGN) bin/mac-pulse

run: build
	bin/mac-pulse

# TESTFLAGS=-short skips the one test that creates a keychain and signs a binary.
test:
	go test -race $(TESTFLAGS) ./...

# cgo hides compiler warnings, so the C and Objective-C packages are rebuilt with them as errors.
lint:
	test -z "$$(gofumpt -l .)"
	golangci-lint run ./...
	CGO_CFLAGS="$(CGO_CFLAGS) -Wall -Wextra -Werror -Wno-unused-parameter" go build -o /dev/null ./internal/shell ./internal/native

# One source draws the .iconset for the bundle and internal/ui/web/icon.png, the kept file the
# frontend embeds for its own row: after a change to the drawing, this rule is what redraws it.
dist/icon.icns: packaging/icon/main.go
	rm -rf dist/icon.iconset
	mkdir -p dist/icon.iconset
	go run ./packaging/icon dist/icon.iconset internal/ui/web/icon.png
	iconutil -c icns -o $@ dist/icon.iconset

bundle: dist/icon.icns
	rm -rf $(APP)
	mkdir -p $(APP)/Contents/MacOS $(APP)/Contents/Resources
	go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o $(APP)/Contents/MacOS/mac-pulse ./cmd/mac-pulse
	cp packaging/Info.plist $(APP)/Contents/Info.plist
	cp dist/icon.icns $(APP)/Contents/Resources/icon.icns
	cp LICENSE THIRD-PARTY-NOTICES.md $(APP)/Contents/Resources/
	$(SIGN) $(APP)

app: bundle

# The same bundle with an arm64 and an x86_64 slice. The x86_64 slice is only known to start under
# Rosetta: it has not been run on an Intel Mac, where the SMC keys and IOReport channels differ.
universal: bundle
	CGO_ENABLED=1 GOARCH=arm64 CC="clang -arch arm64" go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o dist/mac-pulse-arm64 ./cmd/mac-pulse
	CGO_ENABLED=1 GOARCH=amd64 CC="clang -arch x86_64" go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o dist/mac-pulse-x86_64 ./cmd/mac-pulse
	lipo -create -output $(APP)/Contents/MacOS/mac-pulse dist/mac-pulse-arm64 dist/mac-pulse-x86_64
	rm dist/mac-pulse-arm64 dist/mac-pulse-x86_64
	$(SIGN) $(APP)

install: bundle
	mkdir -p $(HOME)/Applications
	rm -rf $(HOME)/Applications/mac-pulse.app
	cp -R $(APP) $(HOME)/Applications/mac-pulse.app

# dmg and zip build the bundle first, unless KEEP_APP=1 says to pack the one that is there:
# `make release` uses it so that the zip holds the very app that is in the image and was notarized.
BUNDLE := $(if $(KEEP_APP),,bundle)

# A compressed disk image with the app and a link to /Applications to drag it onto.
dmg: $(BUNDLE)
	rm -rf dist/dmg $(DMG)
	mkdir -p dist/dmg
	cp -R $(APP) dist/dmg/
	ln -s /Applications dist/dmg/Applications
	hdiutil create -volname "mac-pulse $(VERSION)" -srcfolder dist/dmg -fs HFS+ -format UDZO -ov $(DMG)
	rm -rf dist/dmg
	$(if $(DEVELOPER_ID),codesign --force --timestamp -s "$(DEVELOPER_ID)" $(DMG))

# ditto, unlike zip, keeps the extended attributes and symlinks the signature depends on.
zip: $(BUNDLE)
	rm -f $(ZIP)
	ditto -c -k --keepParent --sequesterRsrc $(APP) $(ZIP)

checksums:
	cd dist && shasum -a 256 mac-pulse-$(VERSION)-*.dmg mac-pulse-$(VERSION)-*.zip > SHA256SUMS.txt
	cat dist/SHA256SUMS.txt

# Signs an existing bundle again with DEVELOPER_ID; `make bundle DEVELOPER_ID=…` signs as it builds.
sign:
ifeq ($(strip $(DEVELOPER_ID)),)
	@echo "DEVELOPER_ID is empty: the bundle keeps its ad hoc signature"
else
	$(SIGN) $(APP)
	codesign --verify --deep --strict --verbose=2 $(APP)
endif

# After `make dmg`: sends the disk image to Apple, waits for the answer and staples the ticket to the
# image and to the app.
ifeq ($(strip $(NOTARY_PROFILE)),)
notarize:
	@echo "NOTARY_PROFILE is empty: nothing is sent to Apple"
else
notarize:
	xcrun notarytool submit $(DMG) --keychain-profile "$(NOTARY_PROFILE)" --wait
	xcrun stapler staple $(DMG)
	xcrun stapler staple $(APP)
endif

# Everything a release needs, in dist/. One make per step, so the order holds under -j as well.
# The zip is made after notarization so that it holds the stapled app.
release:
	$(MAKE) clean
	$(MAKE) test
	$(MAKE) dmg
	$(MAKE) notarize
	$(MAKE) zip KEEP_APP=1
	$(MAKE) checksums

clean:
	rm -rf bin dist
