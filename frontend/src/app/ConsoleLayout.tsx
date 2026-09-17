import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, NavLink, Outlet, useLocation } from "react-router-dom";
import {
  Activity,
  ArrowLeft,
  ChevronRight,
  ListTodo,
  LogOut,
  Moon,
  Radio,
  Sun,
  Settings,
  Users,
  Upload,
  Music2,
  Menu,
} from "lucide-react";
import { useSession } from "../features/session/useSession";
import { ApiError, requestJson } from "../shared/api/client";
import { Button } from "../shared/ui/Button";
import { Preferences, type Language } from "./preferences";

export default function ConsoleLayout() {
  const location = useLocation();
  const session = useSession(),
    client = useQueryClient();
  const [language, setLanguage] = useState<Language>(() =>
    localStorage.getItem("7gr.language") === "en" ? "en" : "zh",
  );
  const [dark, setDark] = useState(
    () => localStorage.getItem("7gr.theme") === "dark",
  );
  const [username, setUsername] = useState(""),
    [password, setPassword] = useState("");
  const [navOpen, setNavOpen] = useState(false);
  const [scenario, setScenario] = useState("populated");
  useEffect(() => {
    if (import.meta.env.MODE === "mock") {
      void fetch("/__mock/scenario")
        .then((response) => response.json())
        .then((data) => setScenario(data.scenario))
        .catch(() => {});
    }
  }, []);
  const en = language === "en";
  useEffect(() => {
    localStorage.setItem("7gr.language", language);
  }, [language]);
  useEffect(() => {
    localStorage.setItem("7gr.theme", dark ? "dark" : "light");
  }, [dark]);
  const login = useMutation({
    mutationFn: () =>
      requestJson("/api/v1/auth/login", {
        method: "POST",
        body: JSON.stringify({ username, password }),
      }),
    onSuccess: async () => {
      setPassword("");
      await client.cancelQueries();
      client.removeQueries({
        predicate: (query) => query.queryKey[0] !== "me",
      });
      await client.invalidateQueries({ queryKey: ["me"] });
    },
  });
  const logout = useMutation({
    mutationFn: () =>
      requestJson("/api/v1/auth/logout", { method: "POST", body: "{}" }),
    onSuccess: async () => {
      await client.cancelQueries();
      client.clear();
      void session.refetch();
    },
  });
  const isUnauthenticated =
    session.error instanceof ApiError && session.error.status === 401;
  const admin = session.data?.user.role === "SUPER_ADMIN";
  const policy = session.data?.policy;
  const navigation = [
    {
      path: "/admin",
      label: en ? "Overview" : "总览",
      icon: Activity,
      visible: true,
    },
    {
      path: "/admin/profiles",
      label: en ? "Recording profiles" : "录制配置",
      icon: Radio,
      visible: true,
    },
    {
      path: "/admin/recordings",
      label: en ? "Recordings" : "录像管理",
      icon: Radio,
      visible: true,
    },
    {
      path: "/admin/uploads",
      label: en ? "Upload settings" : "上传设置",
      icon: Upload,
      visible:
        admin ||
        policy?.can_edit_bilibili_module ||
        policy?.can_edit_cos_module,
    },
    {
      path: "/admin/songs",
      label: en ? "Songs" : "歌曲分析",
      icon: Music2,
      visible: admin,
    },
    {
      path: "/admin/jobs",
      label: en ? "Jobs" : "任务中心",
      icon: ListTodo,
      visible: true,
    },
    {
      path: "/admin/system",
      label: en ? "System settings" : "系统设置",
      icon: Settings,
      visible: admin,
    },
    {
      path: "/admin/accounts",
      label: en ? "Accounts" : "账号管理",
      icon: Users,
      visible: admin,
    },
    {
      path: "/admin/me",
      label: en ? "My account" : "我的账号",
      icon: ArrowLeft,
      visible: true,
    },
  ];
  useEffect(() => {
    setNavOpen(false);
  }, [location.pathname]);
  useEffect(() => {
    const expired = () => {
      void client.invalidateQueries({ queryKey: ["me"] });
    };
    window.addEventListener("7gr:session-expired", expired);
    return () => window.removeEventListener("7gr:session-expired", expired);
  }, [client]);
  const controls = (
    <div className="flex items-center gap-2">
      <select
        className="console-input"
        aria-label="Language / 语言"
        value={language}
        onChange={(e) => setLanguage(e.target.value as Language)}
      >
        <option value="zh">中文</option>
        <option value="en">English</option>
      </select>
      <Button
        aria-label={en ? "Toggle theme" : "切换主题"}
        onClick={() => setDark(!dark)}
      >
        {dark ? <Sun size={17} /> : <Moon size={17} />}
      </Button>
    </div>
  );
  return (
    <Preferences.Provider value={{ language }}>
      <div
        className="console-root min-h-screen"
        data-theme={dark ? "dark" : "light"}
      >
        {!session.data || isUnauthenticated ? (
          <main className="mx-auto max-w-md px-6 py-16">
            <div className="mb-10 flex items-center justify-between">
              <Link to="/admin" className="font-semibold">
                7GRecorder
              </Link>
              {controls}
            </div>
            {session.isPending ? (
              <p role="status">
                {en ? "Checking session…" : "正在检查登录状态…"}
              </p>
            ) : isUnauthenticated ? (
              <form
                className="console-card space-y-5 p-7"
                onSubmit={(e) => {
                  e.preventDefault();
                  login.mutate();
                }}
              >
                <h1 className="text-2xl font-semibold">
                  {en ? "Welcome back" : "欢迎回来"}
                </h1>
                <label className="grid gap-2 text-sm">
                  {en ? "Username" : "用户名"}
                  <input
                    className="console-input"
                    autoComplete="username"
                    required
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                  />
                </label>
                <label className="grid gap-2 text-sm">
                  {en ? "Password" : "密码"}
                  <input
                    className="console-input"
                    type="password"
                    autoComplete="current-password"
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                  />
                </label>
                <Button disabled={login.isPending} type="submit">
                  {en ? "Sign in" : "登录"}
                </Button>
                {login.isError && (
                  <p role="alert">
                    {en ? "Unable to sign in" : "登录失败，请检查账号和密码"}
                  </p>
                )}
              </form>
            ) : (
              <div role="alert">
                <p>{en ? "Session check failed" : "登录状态检查失败"}</p>
                <Button onClick={() => void session.refetch()}>
                  {en ? "Try again" : "重试"}
                </Button>
              </div>
            )}
          </main>
        ) : (
          <div className="lg:grid lg:min-h-screen lg:grid-cols-[224px_1fr]">
            <aside className="console-sidebar flex flex-col border-b border-border px-5 py-6 lg:border-b-0 lg:border-r">
              <div className="flex items-center justify-between gap-3">
                <Link
                  to="/admin"
                  className="flex items-center gap-3 text-lg font-semibold"
                >
                  <span className="rounded-xl bg-accent p-2 text-white">
                    <Radio size={20} />
                  </span>
                  7GRecorder
                </Link>
                <Button
                  className="lg:hidden"
                  aria-label={en ? "Toggle navigation" : "展开导航"}
                  aria-expanded={navOpen}
                  aria-controls="console-navigation"
                  onClick={() => setNavOpen(!navOpen)}
                >
                  <Menu size={18} />
                </Button>
              </div>
              <p className="mb-5 mt-10 hidden px-3 text-[11px] font-medium tracking-widest text-muted lg:block">
                {en ? "WORKSPACE" : "工作空间"}
              </p>
              <nav
                id="console-navigation"
                className={`${navOpen ? "flex" : "hidden"} mt-4 flex-wrap gap-2 lg:mt-0 lg:flex lg:flex-col`}
              >
                {navigation
                  .filter((item) => item.visible)
                  .map((item) => (
                    <NavLink
                      key={item.path}
                      end
                      className={({ isActive }) =>
                        `console-nav${isActive ? " active" : ""}`
                      }
                      to={item.path}
                    >
                      <item.icon size={17} />
                      {item.label}
                      {item.path === "/admin/jobs" && (
                        <ChevronRight className="ml-auto" size={14} />
                      )}
                    </NavLink>
                  ))}
              </nav>
              <div className="mt-auto hidden pt-16 text-xs leading-6 text-muted lg:block">
                {en
                  ? "Your recordings. Your workspace."
                  : "让每一场直播，都有迹可循。"}
              </div>
            </aside>
            <div className="min-w-0">
              <header className="flex flex-wrap items-center justify-between gap-4 border-b border-border px-5 py-4 lg:px-9">
                <div className="flex items-center gap-2 text-sm">
                  <span className="h-2 w-2 rounded-full bg-accent" />
                  <span>{session.data.user.username}</span>
                  <span className="text-xs text-muted">
                    {session.data.user.role === "SUPER_ADMIN"
                      ? en
                        ? "Administrator"
                        : "管理员"
                      : en
                        ? "Manager"
                        : "用户工作区"}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  {controls}
                  <Button
                    disabled={logout.isPending}
                    aria-label={en ? "Sign out" : "退出登录"}
                    onClick={() => logout.mutate()}
                  >
                    <LogOut size={16} />
                  </Button>
                </div>
              </header>
              {logout.isError && (
                <p role="alert" className="p-4 text-red-600">
                  {en ? "Sign out failed" : "退出失败，请重试"}
                </p>
              )}
              <main className="mx-auto max-w-[1600px] p-5 lg:p-9">
                {location.pathname !== "/admin/jobs" && (
                  <header className="mb-7">
                    <p className="console-eyebrow">7GRECORDER / WORKSPACE</p>
                    <h1 className="mt-2 text-3xl font-semibold tracking-tight">
                      {navigation.find(
                        (item) =>
                          item.path === location.pathname.replace(/\/$/, ""),
                      )?.label ?? (en ? "Workspace" : "工作空间")}
                    </h1>
                  </header>
                )}
                <Outlet />
              </main>
            </div>
          </div>
        )}
        {import.meta.env.MODE === "mock" && (
          <div className="mock-toolbar">
            <span>本地模拟</span>
            <select
              aria-label="模拟场景"
              value={scenario}
              onChange={async (e) => {
                await fetch("/__mock/scenario", {
                  method: "POST",
                  body: e.target.value,
                });
                window.location.reload();
              }}
            >
              <option value="populated">正常数据</option>
              <option value="empty">空数据</option>
              <option value="error">请求失败</option>
              <option value="loading">慢速加载</option>
              <option value="manager">普通用户</option>
              <option value="forbidden">无权访问</option>
              <option value="signedout">未登录</option>
            </select>
          </div>
        )}
      </div>
    </Preferences.Provider>
  );
}
