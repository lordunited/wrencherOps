import { NewRawServiceForm } from "@/components/NewRawServiceForm";

const PLACEHOLDER = `services:
  app:
    image: nginx:latest
    restart: always
    ports:
      - '8080:80'
`;

export default function NewDockerServicePage() {
  return (
    <NewRawServiceForm
      title="New Docker Compose service"
      description="Paste your own docker-compose.yml. It's applied exactly as written — nothing is generated for you."
      endpoint="/api/services/docker"
      basePath="/deploy/docker"
      backLabel="Docker Compose"
      manifestLabel="docker-compose.yml"
      manifestPlaceholder={PLACEHOLDER}
      defaultDeployDir="/opt/app"
    />
  );
}
