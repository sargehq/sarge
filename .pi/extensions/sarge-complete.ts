import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";

export default function (pi: ExtensionAPI) {
  // Primary safety net: when the agent finishes processing, check if the task
  // is still in 'processing' status and call sarge complete if so.
  pi.on("agent_end", async (_event, _ctx) => {
    const taskID = process.env.SARGE_TASK_ID;
    if (!taskID) return;

    try {
      // Check if task is still processing by running sarge task show
      const result = await pi.exec("sarge", ["task", "show", taskID], {
        timeout: 10000,
      });

      // If the task is still in processing status, the agent forgot to complete it
      if (result.stdout.includes("Status:      processing")) {
        console.error(
          `[sarge-complete] Task ${taskID} still processing after agent_end, calling sarge complete...`
        );
        const completeResult = await pi.exec(
          "sarge",
          ["complete", taskID, "--error", "Agent ended without completing task"],
          { timeout: 30000 }
        );
        if (completeResult.code !== 0) {
          console.error(
            `[sarge-complete] sarge complete failed: ${completeResult.stderr}`
          );
        }
      }
    } catch (err) {
      console.error(`[sarge-complete] Error checking/completing task: ${err}`);
    }
  });

  // Secondary safety net: session_shutdown (may not fire in print mode, but
  // covers interactive usage)
  pi.on("session_shutdown", async (_event, _ctx) => {
    const taskID = process.env.SARGE_TASK_ID;
    if (!taskID) return;

    try {
      const result = await pi.exec("sarge", ["task", "show", taskID], {
        timeout: 10000,
      });

      if (result.stdout.includes("Status:      processing")) {
        console.error(
          `[sarge-complete] Task ${taskID} still processing at session_shutdown, calling sarge complete...`
        );
        const completeResult = await pi.exec(
          "sarge",
          ["complete", taskID, "--error", "Agent session ended without completing task"],
          { timeout: 30000 }
        );
        if (completeResult.code !== 0) {
          console.error(
            `[sarge-complete] sarge complete failed: ${completeResult.stderr}`
          );
        }
      }
    } catch (err) {
      console.error(
        `[sarge-complete] Error at session_shutdown: ${err}`
      );
    }
  });
}
