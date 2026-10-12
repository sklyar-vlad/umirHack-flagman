import { updateSessionSettings } from '@/entities/session'
import type { RestaurantSettings } from '@/entities/restaurant-settings'
export function acceptSettings(settings: RestaurantSettings) {
  return updateSessionSettings({ restaurantName: settings.restaurant_name, timezone: settings.timezone, expiringThresholdHours: settings.expiring_threshold_hours, autoWriteoffExpired: settings.auto_writeoff_expired, revision: settings.revision })
}
