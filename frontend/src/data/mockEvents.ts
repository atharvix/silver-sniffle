export interface SocialEvent {
  id: string;
  title: string;
  hostName: string;
  hostAvatar: string;
  distanceMeters: number;
  time: string;
  location: string;
  description: string;
  attendeesCount: number;
  joined?: boolean;
}

export const INITIAL_EVENTS: SocialEvent[] = [
  {
    id: 'e1',
    title: '☕ Coffee & Startup Ideas Catchup',
    hostName: 'Heston Mogotlane',
    hostAvatar: 'https://images.unsplash.com/photo-1534528741775-53994a69daeb?auto=format&fit=crop&q=80&w=1000',
    distanceMeters: 8,
    time: 'Today at 4:30 PM',
    location: 'Building Cafe, Floor 1',
    description: 'Casual catchup for founders, developers, and designers working in health tech and AI.',
    attendeesCount: 4,
    joined: false,
  },
  {
    id: 'e2',
    title: '🎨 AR/VR Design & WebGL Showcase',
    hostName: 'Sophia Lin',
    hostAvatar: 'https://images.unsplash.com/photo-1517841905240-472988babdf9?auto=format&fit=crop&q=80&w=1000',
    distanceMeters: 18,
    time: 'Tomorrow at 6:00 PM',
    location: 'Lounge Area B',
    description: 'Live demo of 3D web graphics, Figma prototypes, and next-gen UI components.',
    attendeesCount: 6,
    joined: true,
  },
  {
    id: 'e3',
    title: '🍕 Late Night Code & Pizza Hangout',
    hostName: 'Alex Rivera',
    hostAvatar: 'https://images.unsplash.com/photo-1539571696357-5a69c17a67c6?auto=format&fit=crop&q=80&w=1000',
    distanceMeters: 25,
    time: 'Tonight at 8:00 PM',
    location: 'Rooftop Lounge',
    description: 'Open co-working and brainstorming session over pizza and drinks for nearby builders.',
    attendeesCount: 3,
    joined: false,
  },
];
