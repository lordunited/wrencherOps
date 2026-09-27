import { TableCard } from "@/components/TableCard";
import { StatusBadge } from "@/components/StatusBadge";
import { getUsers } from "@/lib/api";
import { userStateRole } from "@/lib/format";

export default async function UsersPage() {
  const users = await getUsers();

  return (
    <div className="flex flex-col gap-4">
      <h1 className="page-title">
        Users <span className="text-text-muted">({users.length})</span>
      </h1>
      <TableCard>
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b border-gridline text-left text-text-secondary">
            <th className="py-2 pr-4 font-medium">Username</th>
            <th className="py-2 pr-4 font-medium">Name</th>
            <th className="py-2 font-medium">State</th>
          </tr>
        </thead>
        <tbody>
          {users.map((user) => (
            <tr key={user.id} className="border-b border-gridline">
              <td className="py-2 pr-4">
                <a
                  href={user.web_url}
                  target="_blank"
                  rel="noreferrer"
                  className="text-text-primary hover:underline"
                >
                  {user.username}
                </a>
              </td>
              <td className="py-2 pr-4 text-text-secondary">{user.name}</td>
              <td className="py-2">
                <StatusBadge role={userStateRole(user.state)} label={user.state} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      </TableCard>
    </div>
  );
}
