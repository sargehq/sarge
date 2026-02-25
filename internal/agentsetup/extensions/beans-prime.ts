import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";

export default function (pi: ExtensionAPI) {
  let hasPrimed = false;

  // Run `beans prime` once per session and inject the output as context for the LLM.
  // Uses before_agent_start (not session_start) so we can return a message for the LLM.
  // The hasPrimed guard ensures it only runs on the first prompt, not every prompt.
  pi.on("before_agent_start", async (_event, _ctx) => {
    if (hasPrimed) return;
    hasPrimed = true;

    const result = await pi.exec("beans", ["prime"], { timeout: 10000 });

    // Exit code 1 with no output usually means no beans project — skip silently.
    if (result.code !== 0 || !result.stdout.trim()) {
      return;
    }

    return {
      message: {
        customType: "beans-prime",
        content: result.stdout.trim(),
        display: true,
      },
    };
  });
}
