import React from "react";

function AchievementsPanel({ achievements = [] }) {
  return (
    <div className="achievements-panel">
      <h3>Achievements</h3>
      {achievements.length === 0 ? (
        <p>No achievements yet</p>
      ) : (
        <ul>
          {achievements.map((a) => (
            <li key={a.id}>
              {a.name}
              {a.progress !== undefined && ` (${a.progress})`}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export default AchievementsPanel;
