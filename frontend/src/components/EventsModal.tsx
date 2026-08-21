import React, { useState } from 'react';
import type { SocialEvent } from '../data/mockEvents';
import { X, Calendar, Plus, MapPin, Clock, Users, Check } from 'lucide-react';

interface EventsModalProps {
  isOpen: boolean;
  onClose: () => void;
  events: SocialEvent[];
  onJoinEvent: (eventId: string) => void;
  onCreateEvent: (newEvent: Omit<SocialEvent, 'id' | 'attendeesCount'>) => void;
}

export const EventsModal: React.FC<EventsModalProps> = ({
  isOpen,
  onClose,
  events,
  onJoinEvent,
  onCreateEvent,
}) => {
  const [activeSubTab, setActiveSubTab] = useState<'browse' | 'create'>('browse');
  const [title, setTitle] = useState('');
  const [time, setTime] = useState('');
  const [location, setLocation] = useState('');
  const [description, setDescription] = useState('');

  if (!isOpen) return null;

  const handleCreateSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!title || !location || !time) return;

    onCreateEvent({
      title,
      time,
      location,
      description,
      hostName: 'You (Alex Rivera)',
      hostAvatar: 'https://images.unsplash.com/photo-1539571696357-5a69c17a67c6?auto=format&fit=crop&q=80&w=1000',
      distanceMeters: 0,
      joined: true,
    });

    setTitle('');
    setTime('');
    setLocation('');
    setDescription('');
    setActiveSubTab('browse');
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-md overflow-y-auto">
      <div className="relative w-full max-w-lg h-[80vh] bg-[#121319] rounded-[32px] border border-white/10 p-6 flex flex-col my-auto shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between pb-4 border-b border-white/10">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-full bg-white/10 text-white flex items-center justify-center border border-white/10">
              <Calendar className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-white tracking-tight">Events & Meets</h2>
              <p className="text-xs text-neutral-400">Plan a social event or join people nearby</p>
            </div>
          </div>

          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full bg-white/5 hover:bg-white/10 text-neutral-400 hover:text-white flex items-center justify-center transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Subtab Toggle (Browse vs Plan Event) */}
        <div className="flex items-center gap-2 pt-4">
          <button
            type="button"
            onClick={() => setActiveSubTab('browse')}
            className={`flex-1 py-2 rounded-xl text-xs font-semibold transition-all ${
              activeSubTab === 'browse'
                ? 'bg-[#f2ece1] text-neutral-950 shadow-sm'
                : 'bg-white/[0.04] text-neutral-400 hover:text-white border border-white/5'
            }`}
          >
            Browse Events ({events.length})
          </button>

          <button
            type="button"
            onClick={() => setActiveSubTab('create')}
            className={`flex-1 py-2 rounded-xl text-xs font-semibold flex items-center justify-center gap-1.5 transition-all ${
              activeSubTab === 'create'
                ? 'bg-[#f2ece1] text-neutral-950 shadow-sm'
                : 'bg-white/[0.04] text-neutral-400 hover:text-white border border-white/5'
            }`}
          >
            <Plus className="w-3.5 h-3.5" />
            <span>+ Plan an Event</span>
          </button>
        </div>

        {/* Content Body */}
        <div className="flex-1 overflow-y-auto pt-4 space-y-3">
          {activeSubTab === 'browse' ? (
            events.length === 0 ? (
              <div className="h-full flex flex-col items-center justify-center text-center p-6 gap-3">
                <Calendar className="w-8 h-8 text-neutral-600" />
                <p className="text-sm font-semibold text-neutral-300">No Nearby Events</p>
                <p className="text-xs text-neutral-500">Be the first to host an event or coffee catchup!</p>
              </div>
            ) : (
              events.map((ev) => (
                <div
                  key={ev.id}
                  className="p-4 rounded-2xl bg-white/[0.03] hover:bg-white/[0.06] border border-white/10 space-y-3 transition-colors"
                >
                  <div className="flex items-start justify-between">
                    <div>
                      <h4 className="text-sm font-bold text-white">{ev.title}</h4>
                      <div className="flex items-center gap-2 text-[11px] text-neutral-400 mt-1">
                        <img
                          src={ev.hostAvatar}
                          alt={ev.hostName}
                          className="w-4 h-4 rounded-full object-cover"
                        />
                        <span>{ev.hostName}</span>
                        <span>• {ev.distanceMeters === 0 ? 'Your Event' : `${ev.distanceMeters}m away`}</span>
                      </div>
                    </div>

                    <button
                      onClick={() => onJoinEvent(ev.id)}
                      className={`px-3 py-1.5 rounded-xl text-xs font-semibold flex items-center gap-1 transition-all ${
                        ev.joined
                          ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                          : 'bg-white text-neutral-950 hover:bg-[#f2ece1]'
                      }`}
                    >
                      {ev.joined ? (
                        <>
                          <Check className="w-3.5 h-3.5" />
                          <span>Joined</span>
                        </>
                      ) : (
                        <span>Join Event</span>
                      )}
                    </button>
                  </div>

                  <p className="text-xs text-neutral-300 leading-relaxed font-normal">{ev.description}</p>

                  <div className="flex items-center justify-between text-[11px] text-neutral-400 pt-1 border-t border-white/5">
                    <div className="flex items-center gap-3">
                      <span className="flex items-center gap-1">
                        <Clock className="w-3 h-3 text-neutral-500" />
                        {ev.time}
                      </span>
                      <span className="flex items-center gap-1">
                        <MapPin className="w-3 h-3 text-rose-400" />
                        {ev.location}
                      </span>
                    </div>

                    <span className="flex items-center gap-1 font-semibold text-neutral-300">
                      <Users className="w-3 h-3 text-neutral-500" />
                      {ev.attendeesCount} going
                    </span>
                  </div>
                </div>
              ))
            )
          ) : (
            /* Plan an Event Form */
            <form onSubmit={handleCreateSubmit} className="space-y-4 pt-1">
              <div>
                <label className="text-xs font-semibold text-neutral-400 block mb-1">Event Title</label>
                <input
                  type="text"
                  required
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  placeholder="e.g. ☕ Coffee & Founder Chat"
                  className="w-full bg-white/[0.04] border border-white/10 rounded-xl px-3.5 py-2.5 text-xs text-white focus:border-white/30 focus:outline-none"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-xs font-semibold text-neutral-400 block mb-1">Time</label>
                  <input
                    type="text"
                    required
                    value={time}
                    onChange={(e) => setTime(e.target.value)}
                    placeholder="Today at 5:00 PM"
                    className="w-full bg-white/[0.04] border border-white/10 rounded-xl px-3.5 py-2.5 text-xs text-white focus:border-white/30 focus:outline-none"
                  />
                </div>

                <div>
                  <label className="text-xs font-semibold text-neutral-400 block mb-1">Location</label>
                  <input
                    type="text"
                    required
                    value={location}
                    onChange={(e) => setLocation(e.target.value)}
                    placeholder="Building Lobby / Cafe"
                    className="w-full bg-white/[0.04] border border-white/10 rounded-xl px-3.5 py-2.5 text-xs text-white focus:border-white/30 focus:outline-none"
                  />
                </div>
              </div>

              <div>
                <label className="text-xs font-semibold text-neutral-400 block mb-1">Short Event Description</label>
                <textarea
                  rows={3}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="What is the event about? Who should join?"
                  className="w-full bg-white/[0.04] border border-white/10 rounded-xl p-3 text-xs text-white focus:border-white/30 focus:outline-none"
                />
              </div>

              <button
                type="submit"
                className="w-full bg-[#f2ece1] hover:bg-white text-neutral-950 font-bold text-xs py-3 rounded-xl shadow-lg transition-all"
              >
                Publish & Host Event
              </button>
            </form>
          )}
        </div>
      </div>
    </div>
  );
};
