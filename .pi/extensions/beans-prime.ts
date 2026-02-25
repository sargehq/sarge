import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";

export default function (pi: ExtensionAPI) {
  let hasPrimed = false;

  // Run `beans prime` once at the start of the session and inject the output as context for the LLM.
  // This replicates what the beans Claude Code hook does automatically.
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
