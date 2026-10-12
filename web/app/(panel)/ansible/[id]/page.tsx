// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { PlaybookEditor } from "@/components/PlaybookEditor";
import { getAnsibleRun, getAnsibleRuns, getPlaybook, getWatchedServers } from "@/lib/api";

export default async function PlaybookPage(props: PageProps<"/ansible/[id]">) {
  const { id } = await props.params;
  const [playbook, servers, runs] = await Promise.all([getPlaybook(id), getWatchedServers(), getAnsibleRuns(id)]);
  // A run still going when the page opens is shown live, with its log.
  const running = runs.find((r) => r.status === "running");
  if (running) runs[runs.indexOf(running)] = await getAnsibleRun(running.id);

  return (
    <PlaybookEditor
      key={playbook.id}
      initial={playbook}
      servers={servers.map((s) => ({ id: s.id, name: s.name, host: s.host, username: s.username, reachable: !s.checkError }))}
      initialRuns={runs}
    />
  );
}
