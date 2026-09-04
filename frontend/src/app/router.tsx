import { createBrowserRouter, Navigate } from "react-router-dom";
import { AdminDashboard } from "../pages/AdminDashboard";
import { PublicStreamer } from "../pages/PublicStreamer";

export const router = createBrowserRouter([
  {
    path: "/",
    element: <Navigate to="/admin" replace />
  },
  {
    path: "/admin/*",
    element: <AdminDashboard />
  },
  {
    path: "/@:slug/*",
    element: <PublicStreamer />
  }
]);
