import { describe, it } from "vitest";

// Acceptance-criteria stubs from docs/specs/messenger-channels.md that can
// only be checked in the UI. Everything checkable at the API level lives in
// the Go tests messenger_channels_spec*_test.go. The component does not exist
// yet: the section ships together with a working API, with no empty screen
// before that. When implementing, follow AdminAIGatewaysSection.test.tsx.
describe("AdminMessengerChannelsSection (SPEC messenger-channels, not implemented)", () => {
  it.todo(
    "SPEC messenger-channels UI-1: token field is empty with the saved mask as placeholder; saving without typing does not send botToken",
  );
  it.todo(
    "SPEC messenger-channels UI-2: enabling a second bot of the same messenger asks to confirm that users will have to reconnect notifications",
  );
});
