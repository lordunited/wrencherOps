// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { NewComposeAppForm } from "@/components/NewComposeAppForm";
import { getServices, getSigNozTemplate } from "@/lib/api";
import { knownServers } from "@/lib/servers";

// A new SigNoz is a custom compose app started from the SigNoz template:
// its compose, config files and .env (with a password made for it) are
// filled in and stay editable.
export default async function NewSigNozPage(props: PageProps<"/deploy/signoz/new">) {
  const { server } = await props.searchParams;
  const [services, template] = await Promise.all([getServices(), getSigNozTemplate()]);

  return <NewComposeAppForm known={knownServers(services)} preselect={Array.isArray(server) ? server[0] : server} template={template} />;
}
