# Backend Agent Context

This folder contains minimal context for the backend portion of the Context Bridge project.

## ⚠️ Note

The primary focus of this project is the **frontend** (Vue 3 + Composition API + PrimeVue). The backend context is provided for reference but is not the main development area.

## Backend Stack

Based on the project structure, the backend appears to use:
- **Language**: Likely Go, Node.js, or similar
- **Database**: Supabase (PostgreSQL)
- **WebRTC**: Custom WebRTC signaling server

## Backend Location

```
/root/projects/context-bridge/backend/
```

## Integration Points

The frontend communicates with the backend through:

1. **Supabase** (Primary):
   - Authentication (Google OAuth, Email/Password)
   - Database queries (companies, members, rooms, feedback)
   - Real-time subscriptions (if used)

2. **Custom API Endpoints**:
   - `https://jo.vldo.in/start` - WebRTC connection initialization
   - `https://jo.vldo.in/health-check` - Server health monitoring

3. **WebRTC Signaling**:
   - Custom WebRTC implementation for video/audio conferencing
   - Data channels for real-time communication

## Database Schema (Inferred)

### Tables
- `users` - User profiles (managed by Supabase Auth)
- `companies` - Company/workspace information
- `company_members` - Many-to-many relationship between users and companies
- `rooms` - Conference rooms with access control
- `feedback` - User feedback submissions

### Key Relationships
- Users can belong to multiple companies
- Companies have multiple members with roles (admin/member)
- Rooms belong to companies and have access lists

## API Patterns

See the frontend context for API integration patterns:
- `/root/projects/context-bridge/front-end/agent_context/api-patterns.md`

## For More Information

Since the frontend is the primary focus, refer to the comprehensive frontend context:
```
/root/projects/context-bridge/front-end/agent_context/
```
